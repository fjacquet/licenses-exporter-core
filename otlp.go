package licenses_core

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OTLPConfig configures the optional OTLP/gRPC push exporter. Endpoint empty
// disables OTLP entirely (the exporter then serves Prometheus only).
type OTLPConfig struct {
	Endpoint string `yaml:"endpoint"`
	Insecure bool   `yaml:"insecure"`
}

// otlpPushInterval is the periodic reader cadence. License data is near-static;
// the push cadence only affects freshness of the cached snapshot downstream.
const otlpPushInterval = 60 * time.Second

// allMetricNames is the fixed set of observable gauges we register.
var allMetricNames = []string{
	MetricSeatsTotal, MetricSeatsUsed, MetricExpiration,
	MetricUp, MetricLastSuccess, MetricScrapeDuration, MetricBuildInfo,
}

// RegisterOTLP registers one observable gauge per metric name. Each callback
// reads the current snapshot and observes its matching samples at OBSERVATION
// time (points are not back-dated; data age is carried by
// license_collector_last_success_timestamp_seconds).
func RegisterOTLP(meter metric.Meter, store *SnapshotStore) error {
	for _, name := range allMetricNames {
		g, err := meter.Float64ObservableGauge(name, metric.WithDescription(helpText[name]))
		if err != nil {
			return err
		}
		_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
			snap := store.Load()
			if snap == nil {
				return nil
			}
			for _, s := range snap.Samples {
				if s.Name != name {
					continue
				}
				attrs := make([]attribute.KeyValue, len(s.Labels))
				for i, l := range s.Labels {
					attrs[i] = attribute.String(l.Key, l.Value)
				}
				o.ObserveFloat64(g, s.Value, metric.WithAttributes(attrs...))
			}
			return nil
		}, g)
		if err != nil {
			return err
		}
	}
	return nil
}

// Resource builds the OTLP resource attributes for the exporter.
func Resource(version, instanceID string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("licenses_exporter"),
		semconv.ServiceVersion(version),
		semconv.ServiceInstanceID(instanceID),
	)
}

// setupOTLP wires the OTLP/gRPC push exporter when cfg.Endpoint is set. It returns
// a shutdown func that is ALWAYS non-nil (a no-op when OTLP is disabled), so callers
// can defer it unconditionally.
func setupOTLP(ctx context.Context, cfg OTLPConfig, version, instanceID string, store *SnapshotStore) (func(context.Context) error, error) {
	if cfg.Endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(otlpPushInterval))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(Resource(version, instanceID)),
		sdkmetric.WithReader(reader),
	)

	if err := RegisterOTLP(mp.Meter("licenses_exporter"), store); err != nil {
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("register otlp gauges: %w", err)
	}
	return mp.Shutdown, nil
}
