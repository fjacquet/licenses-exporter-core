package licenses_core

import (
	"context"
	"errors"
	"testing"
)

// fakeSource is a deterministic Source for engine tests — no vendor SDK.
type fakeSource struct {
	vendor, instance string
	samples          []Sample
	err              error
}

func (f *fakeSource) Vendor() string   { return f.vendor }
func (f *fakeSource) Instance() string { return f.instance }
func (f *fakeSource) Collect(context.Context) ([]Sample, error) {
	return f.samples, f.err
}

// gatedSource is a fake Source with deterministic coordination hooks: started
// signals (once) when Collect is entered, and release (when non-nil) blocks
// Collect until closed/received — letting a test observe the shared store
// while a reload's first CollectOnce is mid-flight.
type gatedSource struct {
	vendor, instance string
	samples          []Sample
	started          chan struct{} // buffered(1); nil to skip
	release          chan struct{} // nil => return immediately
}

func (g *gatedSource) Vendor() string   { return g.vendor }
func (g *gatedSource) Instance() string { return g.instance }
func (g *gatedSource) Collect(ctx context.Context) ([]Sample, error) {
	if g.started != nil {
		select {
		case g.started <- struct{}{}:
		default:
		}
	}
	if g.release != nil {
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return g.samples, nil
}

func TestFakeSourceImplementsSource(t *testing.T) {
	var _ Source = (*fakeSource)(nil)

	wantSamples := []Sample{{Name: "license_up"}}
	wantErr := errors.New("boom")
	f := &fakeSource{vendor: "acme", instance: "tenant-1", samples: wantSamples, err: wantErr}

	if got := f.Vendor(); got != "acme" {
		t.Fatalf("Vendor() = %q, want %q", got, "acme")
	}
	if got := f.Instance(); got != "tenant-1" {
		t.Fatalf("Instance() = %q, want %q", got, "tenant-1")
	}
	gotSamples, gotErr := f.Collect(context.Background())
	if len(gotSamples) != 1 || gotSamples[0].Name != "license_up" {
		t.Fatalf("Collect() samples = %v, want %v", gotSamples, wantSamples)
	}
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("Collect() err = %v, want %v", gotErr, wantErr)
	}
}
