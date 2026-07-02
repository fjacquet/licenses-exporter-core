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
