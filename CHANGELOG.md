# Changelog

All notable changes to this module are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this module follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) with an API-settling
`0.x` window (promoted to `v1.0.0` once a second independent consumer compiles
against it unchanged).

## [Unreleased]

### Added

- `${VAR:-default}` fallbacks in config env references, ported from `pscale_exporter`.
  Shell / docker-compose semantics: the variable falls back when unset *or* empty, and
  such a reference never aborts startup. A bare `${VAR}` still fails loudly, which is
  what protects secrets from resolving to an empty string.

## [1.1.1] — 2026-08-01

Documentation-only release — no code changed since `v1.1.0`. `v1.1.0`'s own
entry below is left exactly as published (that tag is immutable and already
cached by `proxy.golang.org`); this entry discloses two things that were
already present in `v1.1.0`'s code (commit `ecb5643`, which landed before the
`v1.1.0` tag) but were missing from that release's notes. **Do not read the
absence of a `### Security` section under `[1.1.0]` as "no fix included" — the
fix shipped there, not here.**

### Security

- **GO-2026-6061** (grpc-go, xDS RBAC authorization engine and the HTTP/2
  transport server), closed by `google.golang.org/grpc` `v1.82.0` → `v1.83.0`.
  `govulncheck` flagged this via `setupOTLP`'s gRPC exporter path;
  before/after `govulncheck` runs confirmed the finding is gone. **This fix is
  in `v1.1.0`, not deferred to `v1.1.1`** — a consumer still on `v1.0.1` needs
  `>= v1.1.0` to close it, and a consumer already on `v1.1.0` does not need to
  re-bump for this alone.

### Changed

- **Dependency bump, disclosed retroactively for `v1.1.0`.** The same commit
  that closed GO-2026-6061 also carried the rest of the grpc-adjacent set
  forward: `github.com/go-logr/logr` v1.4.3 → v1.4.4,
  `github.com/prometheus/common` v0.70.0 → v0.70.1,
  `go.opentelemetry.io/proto/otlp` v1.10.0 → v1.11.0, and the
  `google.golang.org/genproto/googleapis/{api,rpc}` pseudo-versions. None
  carry a known vulnerability on their own; recorded here for completeness.
- **`github.com/prometheus/client_golang` v1.23.2 → v1.24.1**, a *direct*
  dependency bump that rode along in the same commit and is **not required**
  for GO-2026-6061 (grpc's fixed-in version is `v1.82.1`; the branch took
  `v1.83.0`). All three consumers (`m365`, `vmware`, `veeam` licenses
  exporters) inherit it via MVS. Verified against the upstream
  `client_golang` `CHANGELOG.md` (v1.24.0/v1.24.1) and source in the module
  cache — both claimed behaviour changes are real:
  - `promhttp` HTTP handlers built via `promhttp.HandlerFor`/`Handler()` now
    accept one or more `name[]` query parameters to filter which metrics are
    returned; the default with no `name[]` given is unchanged (all metrics).
    This module's `/metrics` route (`server.go`) uses plain
    `promhttp.HandlerFor(reg, promhttp.HandlerOpts{})`, so the filter is live
    on all three consumers' `/metrics` endpoints with no code change on
    either side — a new, externally-reachable query capability.
  - Metric/label name validation now always uses the UTF-8 validation scheme
    internally instead of consulting the deprecated
    `model.NameValidationScheme` global; code that set
    `NameValidationScheme = LegacyValidation` no longer gets legacy
    enforcement. This module does not set that global, so no observable
    change here — noted in case a consumer does.
  - Both are satisfied on Go 1.25+: all four family repos declare
    `go 1.26.5`, clearing client_golang v1.24's Go 1.25 floor.

## [1.1.0] — 2026-08-01

### Added

- **`/livez` and `/readyz`** (`server.go`): two fixed routes wired to a
  `staticOKHandler` that reads no state and answers 200 as soon as the listener
  is bound. These are what container `HEALTHCHECK`s and Kubernetes probes should
  target — never `/metrics`, which renders the whole exposition per probe tick and
  can block behind a slow collection cycle.

### Changed

- **`/health` now always answers 200** (`health.go`). The readiness flag stays as
  the *body* (`starting` before the first collection cycle completes, `ok` after)
  and is no longer the status code. Previously it returned 503 `starting` until
  the first cycle, which made a Docker `HEALTHCHECK` report the container
  unhealthy for the whole start-up window and made a Kubernetes `livenessProbe`
  restart a process that was merely still collecting. `SetReady()` and its call
  site are unchanged.

  This is behavioural, not an API break: no exported symbol changed, so consumers
  bump with a plain `go get`. Anything asserting on a 503 from `/health` — an
  alert rule, a smoke test, a blackbox-exporter check — must be updated.

## [1.0.1] — 2026-07-12

### Changed

- **Go toolchain 1.26.4 → 1.26.5** (`go.mod`), with the transitive dependency
  set refreshed alongside it: `golang.org/x/sync` v0.21.0 → v0.22.0,
  `github.com/prometheus/common` v0.66.1 → v0.70.0,
  `github.com/prometheus/procfs` v0.16.1 → v0.21.1,
  `golang.org/x/net` v0.55.0 → v0.57.0, `golang.org/x/sys` v0.45.0 → v0.47.0,
  `golang.org/x/text` v0.37.0 → v0.40.0,
  `go.opentelemetry.io/proto/otlp` v1.10.0, and
  `google.golang.org/grpc` v1.81.1; `go.yaml.in/yaml/v2` dropped as a
  transitive dependency.

### Build

- **`sbom` Make target** (CycloneDX), required by the shared `go-ci.yml`
  workflow used across the exporter family.
- **`coverage-upload` Make target**, completing the shared `go-ci` interface.

## [1.0.0] — 2026-07-02

Stable API. No code changes from `v0.1.0` — this release marks the public API as
stable after a **second independent consumer** (`vmware_licenses_exporter`) compiled
against it unchanged, joining the first (`m365_licenses_exporter`). The `Source`
seam, `Sample` constructors, `Base`/`LoadYAML`, `Server`, and `App`/`Main` entry
point are now covered by semantic-versioning stability guarantees.

## [0.1.0] — 2026-07-02

Initial release: the vendor-neutral engine extracted from `licenses_exporter`
v1.0.0 into a reusable library so that licensing can be exported by one thin
per-vendor exporter per corporation, all sharing a single `license_` schema.

### Added

- **`license_` schema + constructors** (`sample.go`, `metrics.go`): `Sample`/`Label`,
  the seven `license_*` metric-name constants, and the sample constructors
  (`SeatSample`, `ExpirationSample`, and the engine's health/`build_info` builders).
  A golden test locks each constructor's ordered label-key set — the guarantee that
  every vendor emits an identical schema.
- **`Source` seam** (`source.go`): the vendor extension point —
  `Vendor()`/`Instance()`/`Collect(ctx) ([]Sample, error)`.
- **Immutable snapshot store** (`snapshot.go`): `Snapshot`, `ColdStartSnapshot`, and an
  `RWMutex` pointer-swap `SnapshotStore` (`NewSnapshotStore`/`Load`/`Swap`).
- **Collection loop** (`collector.go`): `NewCollector` + `CollectOnce`/`RunTicker`/`Run`,
  errgroup fan-out with per-source graceful degradation to `license_up=0`.
- **Dual export**: an unchecked Prometheus collector (`prometheus.go`, `NewPromCollector`)
  and an OTLP observable-gauge push (`otlp.go`, `setupOTLP`/`RegisterOTLP`), gated on
  `otlp.endpoint`. Both read the same snapshot.
- **Config primitives** (`config.go`, `dotenv.go`): `Base` (`collection` + `otlp`),
  `CollectionConfig` (duration-string interval), `OTLPConfig` (`endpoint`, `insecure`),
  strict `${ENV}` `Expand` (fails on unset), `ResolveSecret` (inline/file), the generic
  `LoadYAML(path, into)`, and `LoadDotEnv` (real env always wins over `.env`).
- **HTTP server + cancelable hot reload** (`health.go`, `server.go`): `NewServer` builds the
  serving stack (shared store, Prometheus registry, OTLP, `/metrics`, `/health`, one bound
  listener) once; `ReloadLoop` swaps only the collection loop on `SIGHUP`/file-change,
  validating each candidate before tearing down the running loop — `/metrics` never blanks,
  the socket never rebinds, and a bad reload keeps the last-good snapshot.
- **Entry point** (`run.go`): `App` + `Main` (the `--once`/serve/reload lifecycle),
  `RunOnce`, and a testable `signalAdapter`. Flag parsing stays in the consumer; `Main`
  takes a parsed `App`. See the README for the ~30-line consumer contract.

### Changed (vs the `licenses_exporter` v1.0.0 engine it was extracted from)

- **Startup now fails fast when the initial config load or source-build fails.** In the
  original, an initial `BuildSources` failure surfaced only *after* the HTTP listener had
  bound, leaving the process serving cold-start `license_build_info` with `/health=503`.
  Here, source construction happens in the consumer's `App.Load`, which `Main` calls before
  binding — so an unbuildable (but syntactically valid) config is fatal at startup instead.
  The reload path is unaffected and strictly more robust: a source-build failure on reload is
  now *rejected*, keeping the last-good collection loop running (the original silently killed
  the loop, leaving a stale snapshot with no active collector).

### Deferred to a future release (tracked; none block `v0.1.0`)

- Extend the golden label-key test to all seven metric names (currently locks the two
  vendor-facing ones; the four engine-emitted ones are pinned indirectly).
- Doc-comment every exported symbol and re-enable revive `exported`/`package-comments`
  before promoting to `v1.0.0`.
- Trim the exported surface (`Server`/`NewServer`/`RunCollection`/`ReloadLoop`/`Collector`/
  `NewCollector`) once the first consumer confirms it only needs `Main` + the
  sample/store/registry seam.

[1.1.1]: https://github.com/fjacquet/licenses-exporter-core/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/fjacquet/licenses-exporter-core/compare/v1.0.1...v1.1.0
[1.0.1]: https://github.com/fjacquet/licenses-exporter-core/compare/v1.0.0...v1.0.1
