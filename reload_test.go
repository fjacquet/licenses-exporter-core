package licenses_core

import (
	"errors"
	"testing"
	"time"
)

// hasUp reports whether snap carries a license_up sample for vendor (any value).
func hasUp(snap *Snapshot, vendor string) bool {
	if snap == nil {
		return false
	}
	for _, s := range snap.Samples {
		if s.Name != MetricUp {
			continue
		}
		for _, l := range s.Labels {
			if l.Key == "vendor" && l.Value == vendor {
				return true
			}
		}
	}
	return false
}

// eventually polls cond until true or the deadline elapses. It waits for a
// convergent condition (a publish that WILL happen), not a guessed race window.
func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout: %s", msg)
}

func baseWithInterval(d time.Duration) Base {
	return Base{Collection: CollectionConfig{Interval: d}}
}

// TestReloadLoopStoreContinuity drives Server.ReloadLoop with fake sources and
// fake reload/shutdown triggers, asserting the ADR-0008 invariants:
//  1. after startup the shared store serves the collected samples (not build_info-only);
//  2. across a reload the store CONTINUES to serve the prior snapshot until the new
//     collection's first CollectOnce publishes (never blanks to build_info-only);
//  3. a reload whose config fails to load is rejected — store/server keep the old data;
//  4. the loop returns cleanly on a shutdown trigger (and never calls os.Exit).
func TestReloadLoopStoreContinuity(t *testing.T) {
	const longInterval = time.Hour // ticker must not fire during the test

	// Source A collects instantly; source B blocks in Collect until released.
	srcA := &gatedSource{vendor: "venA", instance: "instA", samples: []Sample{UpSample("venA", "instA", true)}}
	releaseB := make(chan struct{})
	srcB := &gatedSource{
		vendor: "venB", instance: "instB",
		samples: []Sample{UpSample("venB", "instB", true)},
		started: make(chan struct{}, 1),
		release: releaseB,
	}

	srv := &Server{
		store:   NewSnapshotStore(ColdStartSnapshot("v", "go")),
		health:  &Health{},
		version: "v",
	}

	// load() is driven by the test: each call reports it ran, then returns the
	// next queued (base, sources, err) result.
	type loadResult struct {
		base    Base
		sources []Source
		err     error
	}
	loadResults := make(chan loadResult, 8)
	loadInvoked := make(chan struct{}, 8)
	load := func() (Base, []Source, error) {
		loadInvoked <- struct{}{}
		r := <-loadResults
		return r.base, r.sources, r.err
	}

	reloads := make(chan struct{}, 1)
	shutdown := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		srv.ReloadLoop(baseWithInterval(longInterval), []Source{srcA}, reloads, shutdown, load)
		close(done)
	}()

	// (1) Startup: the store serves A's samples, not just build_info.
	eventually(t, func() bool { return hasUp(srv.store.Load(), "venA") },
		"store never served source A after startup")
	if srv.store.Load() == nil || !srv.health.ready.Load() {
		t.Fatal("health should be ready after the first collection cycle")
	}

	// (2) Reload to B. Queue B's source set and a valid base, then trigger.
	loadResults <- loadResult{base: baseWithInterval(longInterval), sources: []Source{srcB}}
	reloads <- struct{}{}
	<-loadInvoked  // ReloadLoop consumed the reload and validated the candidate
	<-srcB.started // B is now blocked inside Collect: the new snapshot has NOT published

	// The store must STILL serve A (never blank to build_info-only) mid-reload.
	if snap := srv.store.Load(); !hasUp(snap, "venA") || hasUp(snap, "venB") {
		t.Fatalf("mid-reload store must still serve prior snapshot (A), got venA=%v venB=%v",
			hasUp(snap, "venA"), hasUp(snap, "venB"))
	}
	if !srv.health.ready.Load() {
		t.Fatal("health must stay ready across a reload")
	}

	// Release B; the new collection publishes and the store swaps to B.
	close(releaseB)
	eventually(t, func() bool { return hasUp(srv.store.Load(), "venB") },
		"store never swapped to source B after release")
	if hasUp(srv.store.Load(), "venA") {
		t.Fatal("after B published, store must no longer serve A")
	}

	// (3) Reject a bad reload: load() errors → store/server keep serving B.
	loadResults <- loadResult{err: errors.New("bad config")}
	reloads <- struct{}{}
	<-loadInvoked // ReloadLoop consumed and rejected the candidate
	if snap := srv.store.Load(); !hasUp(snap, "venB") {
		t.Fatal("rejected reload must leave the store serving the last-good snapshot (B)")
	}

	// (4) Shutdown returns cleanly (no os.Exit — that would kill this test process).
	shutdown <- struct{}{}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ReloadLoop did not return on shutdown trigger")
	}
}
