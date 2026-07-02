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

The consumer contract (a ~30-line vendor `main.go`) is documented before the
`v0.1.0` tag.

## License

Apache-2.0 — see [LICENSE](LICENSE).
