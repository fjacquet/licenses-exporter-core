# licenses-exporter-core

The vendor-neutral engine for the `licenses_exporter` family: the generic
`license_` metric schema and its constructors, an immutable snapshot store, the
collection loop, the Prometheus + OTLP export paths, and the HTTP server with
cancelable, validated hot reload.

Vendor exporters (VMware, Microsoft 365, Veeam, …) import this module, implement
a single `Source`, and call `Main`. Because every vendor emits the identical
`license_` schema — built only through this module's constructors — N exporters
land in one Prometheus and keep a single cross-vendor Grafana / alerting view.

```go
import core "github.com/fjacquet/licenses-exporter-core"
```

- **Schema identity** is guaranteed by construction: `Sample`s are built only by
  core constructors, and a golden test locks each metric name's label-key set.
- **Library, not a binary.** This module ships no `main`, no release artifacts,
  no Docker image — those live in each consuming vendor exporter.

## Consumer contract

A vendor exporter provides three things: a `Source` implementation, a config
struct embedding `core.Base`, and a thin `main` that hands `core.Main` an `App`.

**1. Implement `Source`** — one per configured tenant/instance:

```go
type Source interface {
    Vendor() string   // constant, e.g. "microsoft"
    Instance() string // tenant / vCenter identifier
    Collect(ctx context.Context) ([]core.Sample, error)
}
```

Build every `Sample` through a core constructor (`core.SeatSample`,
`core.ExpirationSample`, …) — never a raw literal. That is what keeps the
`license_` schema identical across vendors.

**2. Embed `core.Base` in the vendor config:**

```go
type Config struct {
    core.Base `yaml:",inline"`               // collection.interval + otlp.{endpoint,insecure}
    M365      M365Config `yaml:"m365"`        // vendor-specific block
}
```

**3. A ~30-line `main`** — parse flags (cobra/pflag/stdlib, your choice), then
build an `App` whose `Load` re-parses config and rebuilds sources. `core.Main`
runs the whole lifecycle (`--once`, or serve `/metrics` + `/health` with
signal + file-watch hot reload):

```go
func main() {
    var cfgPath, addr string
    var once, debug, trace bool
    // ... flag wiring (--config, --web.listen-address, --once, --debug, --trace) ...

    err := core.Main(core.App{
        Version: version, Addr: addr, Once: once, Debug: debug, Trace: trace,
        ConfigPath: cfgPath, // enables file-watch reload; empty => SIGHUP-only
        Load: func() (core.Base, []core.Source, error) {
            var cfg Config
            if err := core.LoadYAML(cfgPath, &cfg); err != nil {
                return core.Base{}, nil, err
            }
            if err := cfg.Base.Validate(); err != nil {
                return core.Base{}, nil, err
            }
            srcs, err := m365.NewSources(cfg.M365) // returns []core.Source
            return cfg.Base, srcs, err
        },
    })
    if err != nil {
        logrus.WithError(err).Fatal("exporter failed")
    }
}
```

`Load` is called at startup and on every reload, so vendor-config changes
hot-reload too. `core.Main` builds the serving stack once and swaps only the
collection loop on reload — `/metrics` never blanks and the socket never rebinds.

## Versioning

`v0.1.x` while the API settles against the first consumer; promoted to `v1.0.0`
once a second independent consumer compiles against it unchanged.

## License

Apache-2.0 — see [LICENSE](LICENSE).
