package licenses_core

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

// App is what a vendor main assembles and hands to Main. Load re-parses the
// vendor's whole config (Base + vendor collector block) and rebuilds the
// []Source; Main calls it once at startup (--once and serve paths alike) and
// ReloadLoop calls it again on every SIGHUP / config-file-change reload.
type App struct {
	Version    string // build version, stamped into license_build_info
	Addr       string // --web.listen-address, e.g. ":9105"
	Once       bool   // --once: run one collection cycle then exit (no server)
	Debug      bool   // gates the --once sample dump; also sets logrus debug level
	Trace      bool   // logs a generic "SDK tracing intentionally not wired" warning
	ConfigPath string // path watched for hot reload; empty => reload via SIGHUP only
	Load       func() (Base, []Source, error)
}

// Main runs the whole exporter lifecycle for App.
//
// --once path: one Load, one RunOnce (dumping samples iff Debug), then return
// — no server is started.
//
// Serve path: Load once, build the process-lifetime server (NewServer), wire
// OS signals and an optional fsnotify watcher into ReloadLoop's plain
// reloads/shutdown channels via signalAdapter, then block in ReloadLoop until
// a shutdown trigger. Only the initial Load and the initial NewServer bind
// are fatal (startup); once serving, a reload failure is logged and skipped —
// it never crashes the process. Flag/config-format parsing stays in the
// CONSUMER: Main takes an already-parsed App.
func Main(app App) error {
	if app.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if app.Trace {
		logrus.Warn("--trace: vendor SDKs are non-injectable, so SDK-level tracing is intentionally not wired (it would leak bearer tokens / session cookies); only repo-owned transports are traced. See the exporter's ADRs.")
	}

	base, sources, err := app.Load()
	if err != nil {
		return err // initial load failure is fatal
	}

	if app.Once {
		return RunOnce(context.Background(), sources, app.Version, app.Debug)
	}

	srv, err := NewServer(base, app.Version, app.Addr)
	if err != nil {
		return err // initial bind / OTLP setup failure is fatal
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	var watcher *fsnotify.Watcher
	if app.ConfigPath != "" {
		w, werr := fsnotify.NewWatcher()
		if werr != nil {
			// File-watch is best-effort: without it, reload still works via SIGHUP.
			logrus.WithError(werr).Warn("config file watcher unavailable; reload via SIGHUP only")
		} else {
			watcher = w
			defer func() { _ = watcher.Close() }()
			if err := watcher.Add(app.ConfigPath); err != nil {
				logrus.WithError(err).WithField("file", app.ConfigPath).Warn("cannot watch config file; reload via SIGHUP only")
			}
		}
	}
	events := watcherEvents(watcher) // hoisted once; rebuilt-per-select was wasteful
	errs := watcherErrors(watcher)   // drained so watcher errors are surfaced, not lost

	// Adapt OS signals + file events into plain reload/shutdown triggers so the
	// reload state machine (ReloadLoop) stays free of signal/fsnotify types and
	// is unit-testable.
	reloads := make(chan struct{}, 1)
	shutdown := make(chan struct{}, 1)
	go signalAdapter(sigs, events, errs, reloads, shutdown)

	srv.ReloadLoop(base, sources, reloads, shutdown, app.Load)
	return nil
}

// RunOnce runs a single collection cycle against a throwaway store — the
// --once path. Samples are dumped (sorted, exposition style) ONLY when debug
// is true. No server is started.
func RunOnce(ctx context.Context, sources []Source, version string, debug bool) error {
	store := NewSnapshotStore(ColdStartSnapshot(version, runtime.Version()))
	collector := NewCollector(sources, store, version, runtime.Version(), 0, time.Now)
	snap := collector.CollectOnce(ctx)
	if debug {
		dumpSamples(snap)
	}
	return nil
}

// dumpSamples prints every sample sorted (exposition style) for --once --debug.
func dumpSamples(snap *Snapshot) {
	lines := make([]string, 0, len(snap.Samples))
	for _, s := range snap.Samples {
		lines = append(lines, fmt.Sprintf("%s %g", s.Name, s.Value))
	}
	sort.Strings(lines)
	for _, l := range lines {
		fmt.Println(l)
	}
}

// signalAdapter translates OS signals + fsnotify events into the plain
// reload/shutdown triggers ReloadLoop consumes: SIGHUP or a watched file
// write/create sends (non-blocking, coalesced) on reloads; SIGINT/SIGTERM
// sends on shutdown and then returns. Watcher errors are logged, not fatal.
// Extracted out of the server-startup goroutine so it is unit-testable
// without real signals or files.
func signalAdapter(sigs <-chan os.Signal, events <-chan fsnotify.Event, errs <-chan error, reloads, shutdown chan<- struct{}) {
	for {
		select {
		case sig := <-sigs:
			if sig == syscall.SIGHUP {
				logrus.Info("SIGHUP: reloading config")
				select {
				case reloads <- struct{}{}:
				default:
				}
			} else {
				select {
				case shutdown <- struct{}{}:
				default:
				}
				return // SIGINT/SIGTERM: stop translating
			}
		case ev := <-events:
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			logrus.WithField("file", ev.Name).Info("config changed: reloading")
			select {
			case reloads <- struct{}{}:
			default:
			}
		case werr := <-errs:
			// Surface watcher errors instead of leaving them to pile up in the
			// channel; file-watch degrades but SIGHUP reload still works.
			logrus.WithError(werr).Warn("config file watcher error")
		}
	}
}

func watcherEvents(w *fsnotify.Watcher) <-chan fsnotify.Event {
	if w == nil {
		return make(chan fsnotify.Event) // never fires
	}
	return w.Events
}

func watcherErrors(w *fsnotify.Watcher) <-chan error {
	if w == nil {
		return make(chan error) // never fires
	}
	return w.Errors
}
