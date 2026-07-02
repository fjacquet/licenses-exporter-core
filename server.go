package licenses_core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Server is the process-lifetime serving stack: one shared SnapshotStore, the
// Prometheus registry+collector, the OTLP push exporter, the /health handler, and
// a single bound HTTP server. It is built ONCE (NewServer) and reused across every
// reload — RunCollection swaps only the collection loop, never the store or the
// listener, so /metrics never blanks and /health never flips back to 503 on a
// reload (ADR-0008 §4).
type Server struct {
	srv          *http.Server
	ln           net.Listener
	store        *SnapshotStore
	health       *Health
	shutdownOTLP func(context.Context) error
	version      string
}

// NewServer builds+starts the process-lifetime serving stack (shared store, Prometheus
// registry+collector, OTLP exporter, /health, one bound listener). Bind failure is
// returned (fatal at startup only); a runtime serve error is LOGGED, never fatal.
func NewServer(base Base, version, addr string) (*Server, error) {
	store := NewSnapshotStore(ColdStartSnapshot(version, runtime.Version()))

	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))

	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "unknown"
	}
	shutdownOTLP, err := setupOTLP(context.Background(), base.OTLP, version, instanceID, store)
	if err != nil {
		return nil, err
	}

	health := &Health{}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.Handle("/health", health)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		_ = shutdownOTLP(context.Background())
		return nil, fmt.Errorf("listen %q: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s := &Server{
		srv:          srv,
		ln:           ln,
		store:        store,
		health:       health,
		shutdownOTLP: shutdownOTLP,
		version:      version,
	}
	go func() {
		logrus.WithField("addr", addr).Info("serving /metrics and /health")
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logrus.WithError(err).Error("http server failed")
		}
	}()
	return s, nil
}

// RunCollection runs one collection into the SHARED store until ctx is canceled:
// build a collector from sources, CollectOnce, SetReady, then RunTicker(interval).
// Exactly one leading collect (no double initial collect). Publishing into the
// existing shared store means the prior snapshot keeps serving until the first
// CollectOnce swaps. Reloads call this repeatedly on the same *Server.
func (s *Server) RunCollection(ctx context.Context, sources []Source, interval time.Duration) error {
	collector := NewCollector(sources, s.store, s.version, runtime.Version(), 0, time.Now)
	collector.CollectOnce(ctx)
	s.health.SetReady()
	collector.RunTicker(ctx, interval)
	return nil
}

// ReloadLoop is the reload state machine. It runs one collection loop under a
// cancelable context, then on each reload trigger validates a candidate (via
// load) and — only if it loads/validates — cancels the running loop and
// respawns collection with the new base/sources on the SAME server/store. A
// candidate that fails to load is logged and skipped: the running collection
// and server are left untouched. A shutdown trigger cancels the active loop
// and returns.
//
// Callers adapt OS signals (SIGHUP → reloads, SIGINT/SIGTERM → shutdown) and
// fsnotify write/create events (→ reloads) into the two trigger channels; keeping
// them as plain channels makes this loop testable without real signals or files.
func (s *Server) ReloadLoop(initialBase Base, initialSources []Source, reloads, shutdown <-chan struct{}, load func() (Base, []Source, error)) {
	base := initialBase
	sources := initialSources
	for {
		ctx, cancel := context.WithCancel(context.Background())
		go func(base Base, sources []Source) {
			if err := s.RunCollection(ctx, sources, base.Collection.Interval); err != nil {
				logrus.WithError(err).Error("collection cycle ended")
			}
		}(base, sources)

		var newBase Base
		var newSources []Source
		haveNew := false
		for !haveNew {
			select {
			case <-shutdown:
				cancel() // stop the active collection loop and exit
				return
			case <-reloads:
				// Validate the candidate BEFORE tearing down the running loop.
				b, srcs, err := load()
				if err != nil {
					logrus.WithError(err).Warn("new config invalid; keeping current running config")
					continue
				}
				newBase = b
				newSources = srcs
				haveNew = true
			}
		}
		cancel() // tear down old loop; outer loop respawns with the new, validated config
		base = newBase
		sources = newSources
	}
}

// Shutdown gracefully stops the HTTP server and the OTLP exporter.
func (s *Server) Shutdown(ctx context.Context) error {
	err := s.srv.Shutdown(ctx)
	if s.shutdownOTLP != nil {
		if oerr := s.shutdownOTLP(ctx); oerr != nil && err == nil {
			err = oerr
		}
	}
	return err
}
