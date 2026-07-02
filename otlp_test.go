package licenses_core

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// TestRegisterOTLPObservesSnapshot asserts dual-export parity: a sample in the
// snapshot store is observed as an OTLP gauge point with the same name/value
// the Prometheus path would expose for it.
func TestRegisterOTLPObservesSnapshot(t *testing.T) {
	store := NewSnapshotStore(&Snapshot{Samples: []Sample{
		SeatSample(MetricSeatsUsed, "microsoft", "M365_E5", "users", "tenant-a", 242),
	}})
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := provider.Meter("licenses_exporter")
	if err := RegisterOTLP(meter, store); err != nil {
		t.Fatalf("RegisterOTLP: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}

	var found bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != MetricSeatsUsed {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("%s is not a float64 gauge", m.Name)
			}
			for _, dp := range g.DataPoints {
				if dp.Value == 242 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("license_seats_used=242 not observed via OTLP ManualReader")
	}
}

func TestSetupOTLPDisabledWhenNoEndpoint(t *testing.T) {
	store := NewSnapshotStore(ColdStartSnapshot("v", "go"))
	shutdown, err := setupOTLP(context.Background(), OTLPConfig{}, "v", "id", store)
	if err != nil {
		t.Fatalf("disabled setup should not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil even when disabled")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown should not error: %v", err)
	}
}

func TestSetupOTLPEnabledConstructsProvider(t *testing.T) {
	store := NewSnapshotStore(ColdStartSnapshot("v", "go"))
	// Construction must succeed without a live collector (grpc dials lazily).
	cfg := OTLPConfig{Endpoint: "127.0.0.1:4317", Insecure: true}
	shutdown, err := setupOTLP(context.Background(), cfg, "v", "id", store)
	if err != nil {
		t.Fatalf("enabled setup should construct without error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil")
	}
	// Bounded shutdown so the test never hangs on a dead endpoint; a dial-timeout
	// error here is acceptable (we only require setup succeeded and shutdown returns).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = shutdown(ctx)
}
