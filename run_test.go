package licenses_core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything written to it. Deterministic: the pipe is fully drained
// (via a goroutine + WaitGroup) after fn returns and stdout is restored, so
// there is no race between the writer and the reader.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf strings.Builder
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&buf, r)
	}()

	fn()

	os.Stdout = orig
	_ = w.Close()
	wg.Wait()
	_ = r.Close()
	return buf.String()
}

// TestRunOnceDumpsWhenDebug proves the --once dump is gated on debug: with
// debug=true the sample line is printed; with debug=false nothing is.
func TestRunOnceDumpsWhenDebug(t *testing.T) {
	src := &fakeSource{vendor: "acme", instance: "i1",
		samples: []Sample{SeatSample(MetricSeatsUsed, "acme", "p", "u", "i1", 3)}}

	out := captureStdout(t, func() {
		if err := RunOnce(context.Background(), []Source{src}, "v", true); err != nil {
			t.Fatalf("RunOnce (debug) returned error: %v", err)
		}
	})
	if !strings.Contains(out, "license_seats_used 3") {
		t.Fatalf("debug=true output = %q, want it to contain %q", out, "license_seats_used 3")
	}

	out = captureStdout(t, func() {
		if err := RunOnce(context.Background(), []Source{src}, "v", false); err != nil {
			t.Fatalf("RunOnce (no debug) returned error: %v", err)
		}
	})
	if out != "" {
		t.Fatalf("debug=false output = %q, want empty", out)
	}
}

// TestMainOnceReturnsLoadError proves Main surfaces a fatal Load error at
// startup (the --once path never runs a collection cycle in that case).
func TestMainOnceReturnsLoadError(t *testing.T) {
	app := App{
		Version: "test",
		Once:    true,
		Load: func() (Base, []Source, error) {
			return Base{}, nil, errors.New("boom")
		},
	}
	err := Main(app)
	if err == nil {
		t.Fatal("Main() returned nil error, want the Load error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Main() error = %v, want it to wrap %q", err, "boom")
	}
}

// TestSignalAdapterSIGHUPTriggersReload proves SIGHUP is translated into a
// non-blocking send on reloads, with no real file watcher involved.
func TestSignalAdapterSIGHUPTriggersReload(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	events := watcherEvents(nil) // never fires
	errs := watcherErrors(nil)   // never fires
	reloads := make(chan struct{}, 1)
	shutdown := make(chan struct{}, 1)

	done := make(chan struct{})
	go func() {
		signalAdapter(sigs, events, errs, reloads, shutdown)
		close(done)
	}()
	defer func() {
		// Drain the adapter goroutine so the test doesn't leak it: a SIGTERM
		// makes it return.
		sigs <- syscall.SIGTERM
		<-done
	}()

	sigs <- syscall.SIGHUP

	select {
	case <-reloads:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reload trigger after SIGHUP")
	}
}

// TestSignalAdapterSIGTERMTriggersShutdownAndReturns proves SIGINT/SIGTERM
// triggers a shutdown send AND stops the adapter goroutine (it must not keep
// consuming signals/events after that).
func TestSignalAdapterSIGTERMTriggersShutdownAndReturns(t *testing.T) {
	sigs := make(chan os.Signal, 1)
	events := watcherEvents(nil)
	errs := watcherErrors(nil)
	reloads := make(chan struct{}, 1)
	shutdown := make(chan struct{}, 1)

	done := make(chan struct{})
	go func() {
		signalAdapter(sigs, events, errs, reloads, shutdown)
		close(done)
	}()

	sigs <- syscall.SIGTERM

	select {
	case <-shutdown:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown trigger after SIGTERM")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("signalAdapter did not return after SIGTERM")
	}
}

// TestServerServesMetricsAndHealth closes T8's NewServer-coverage note: it
// exercises NewServer + RunCollection end to end against a real bound
// listener, proving /health flips 503->200 and /metrics carries
// license_build_info.
func TestServerServesMetricsAndHealth(t *testing.T) {
	srv, err := NewServer(Base{}, "v", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	addr := srv.ln.Addr().String()
	base := "http://" + addr

	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/health before collection = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	src := &fakeSource{vendor: "acme", instance: "i1",
		samples: []Sample{SeatSample(MetricSeatsUsed, "acme", "p", "u", "i1", 3)}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = srv.RunCollection(ctx, []Source{src}, time.Hour)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get(base + "/health")
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/health never became ready, last status = %d", resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}

	resp, err = http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if !strings.Contains(string(body), "license_build_info") {
		t.Fatalf("/metrics body missing license_build_info, got:\n%s", body)
	}
}
