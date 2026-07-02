package licenses_core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadYAMLExpandsEnvAndParsesInterval(t *testing.T) {
	t.Setenv("OTLP_EP", "otel:4317")
	p := writeTemp(t, `
collection:
  interval: 2h
otlp:
  endpoint: ${OTLP_EP}
`)
	var base Base
	if err := LoadYAML(p, &base); err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if base.Collection.Interval != 2*time.Hour {
		t.Fatalf("interval = %v, want 2h", base.Collection.Interval)
	}
	if base.OTLP.Endpoint != "otel:4317" {
		t.Fatalf("otlp.endpoint = %q, want expanded", base.OTLP.Endpoint)
	}
}

func TestLoadYAMLFailsOnUnsetEnv(t *testing.T) {
	p := writeTemp(t, `
collection:
  interval: 1h
otlp:
  endpoint: ${DEFINITELY_UNSET_VAR}
`)
	var base Base
	if err := LoadYAML(p, &base); err == nil {
		t.Fatal("expected error on unset env var")
	}
}

func TestLoadYAMLParsesOTLPSection(t *testing.T) {
	p := writeTemp(t, `
collection:
  interval: 1h
otlp:
  endpoint: otel-collector:4317
  insecure: true
`)
	var base Base
	if err := LoadYAML(p, &base); err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if base.OTLP.Endpoint != "otel-collector:4317" || !base.OTLP.Insecure {
		t.Fatalf("otlp parsed wrong: %+v", base.OTLP)
	}
}

func TestLoadYAMLOTLPAbsentIsDisabled(t *testing.T) {
	p := writeTemp(t, `
collection:
  interval: 1h
`)
	var base Base
	if err := LoadYAML(p, &base); err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if base.OTLP.Endpoint != "" {
		t.Fatalf("expected empty OTLP endpoint, got %q", base.OTLP.Endpoint)
	}
}

func TestBaseValidateRejectsNonPositiveInterval(t *testing.T) {
	if err := (Base{}).Validate(); err == nil {
		t.Fatal("expected error for zero interval")
	}
	valid := Base{Collection: CollectionConfig{Interval: time.Hour}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
}

func TestResolveSecretFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	if err := os.WriteFile(file, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveSecret("inline-value", file)
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "from-file" {
		t.Fatalf("got %q, want trimmed file contents", got)
	}

	got, err = ResolveSecret("inline-value", "")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if got != "inline-value" {
		t.Fatalf("got %q, want inline value", got)
	}
}
