package licenses_core

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// CollectionConfig configures the collection loop's polling cadence.
type CollectionConfig struct {
	Interval time.Duration `yaml:"interval"`
}

// UnmarshalYAML lets collection.interval accept a Go duration string ("2h").
func (c *CollectionConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw struct {
		Interval string `yaml:"interval"`
	}
	if err := unmarshal(&raw); err != nil {
		return err
	}
	d, err := time.ParseDuration(raw.Interval)
	if err != nil {
		return fmt.Errorf("collection.interval %q: %w", raw.Interval, err)
	}
	c.Interval = d
	return nil
}

// Base is the vendor-neutral configuration shared by every exporter built on
// this engine: the collection loop cadence and the optional OTLP push
// exporter. Vendor exporters embed Base alongside their own collector config.
type Base struct {
	Collection CollectionConfig `yaml:"collection"`
	OTLP       OTLPConfig       `yaml:"otlp"`
}

// Validate checks the base-level invariants only; vendor collector config
// (e.g. "at least one collector enabled") is validated by the caller.
func (b Base) Validate() error {
	if b.Collection.Interval <= 0 {
		return fmt.Errorf("collection.interval must be > 0")
	}
	return nil
}

var envRef = regexp.MustCompile(`\$\{([A-Z0-9_]+)(:-[^}]*)?\}`)

// Expand replaces ${VAR} references, failing on any unset variable.
//
// A reference may carry a fallback as ${VAR:-default}, borrowing the shell /
// docker-compose syntax and its meaning: unset OR empty falls back, and the reference
// never errors. That lets a shipped config.yaml drive a non-secret setting from the
// environment while still starting on a host that never exported it. Use it only where a
// safe default exists.
//
// A bare ${VAR} fails when the variable is UNSET; an exported-but-empty one expands
// to the empty string, as it always has.
func Expand(s string) (string, error) {
	var missing string
	out := envRef.ReplaceAllStringFunc(s, func(m string) string {
		sub := envRef.FindStringSubmatch(m)
		name, fallback := sub[1], sub[2]
		v, ok := os.LookupEnv(name)
		if ok && v != "" {
			return v
		}
		if fallback != "" {
			return fallback[len(":-"):] // group 2 keeps its ":-" prefix, so "" means absent
		}
		if !ok {
			missing = name
			return m
		}
		return ""
	})
	if missing != "" {
		return "", fmt.Errorf("config references unset environment variable %q", missing)
	}
	return out, nil
}

// ResolveSecret returns the secret read from file (trimmed of surrounding
// whitespace) when file is set, otherwise the inline value. Shared by the
// vendor collectors so inline-vs-file precedence stays consistent across them.
func ResolveSecret(inline, file string) (string, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return inline, nil
}

// LoadYAML reads .env, expands ${ENV} references, and unmarshals the
// resulting YAML into into. It does not validate; callers run Base.Validate
// (and any vendor-specific validation) after LoadYAML returns.
func LoadYAML(path string, into any) error {
	LoadDotEnv(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	expanded, err := Expand(string(raw))
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(expanded), into); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return nil
}
