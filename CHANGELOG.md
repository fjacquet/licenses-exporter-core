# Changelog

All notable changes to this module are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this module follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) with an API-settling
`0.x` window (promoted to `v1.0.0` once a second independent consumer compiles
against it unchanged).

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
