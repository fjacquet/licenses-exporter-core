# Licenses-family standard catch-up (Plan A) Implementation Plan

> **Note for agentic workers:** this plan is written to be executed by fresh agents
> with no prior context. Work the tasks in the order given — Tasks 1–5 change and
> release `licenses-exporter-core`, and Tasks 6–17 cannot start until the
> `v1.1.0` tag from Task 5 is pushed and resolvable by `go get`. Check off each
> `- [ ]` step as you complete it. Every command in this plan uses absolute paths;
> run them from the repository each task names.

**Goal:** Bring the three licenses exporters (`m365_licenses_exporter`,
`vmware_licenses_exporter`, `veeam_licenses_exporter`) onto the two family
standards they were skipped by: the always-200 `/livez` + `/readyz` probe pattern,
and the Alpine container-image standard with a working `HEALTHCHECK`. Because all
three share their HTTP wiring through `licenses-exporter-core`, the probe half is
**one library change plus three dependency bumps**, not three implementations.

**Architecture:** `licenses-exporter-core` owns the only `http.ServeMux` in the
family (`server.go` `NewServer`, lines 51–54) and the only `/health` handler
(`health.go` `Health.ServeHTTP`, lines 16–24). None of the three consumers
registers an HTTP route of its own — each `main.go` delegates the whole lifecycle
to `core.Main`. So:

- Core grows a `staticOKHandler` wired to `/livez` and `/readyz`, and
  `Health.ServeHTTP` stops using the status code as a readiness gate — the
  `ready` flag becomes body content only (`"starting"` → `"ok"`). `SetReady()`
  and its single call site (`server.go:93`) are untouched.
- Core cuts `v1.1.0` (additive, non-breaking library API).
- Each consumer bumps the dependency, converts its `Dockerfile.goreleaser` from
  `gcr.io/distroless/static:nonroot` to Alpine (uid `65532` → `10001`, named user
  `licenses` — **breaking** for anyone pinning the container uid), gains a
  `HEALTHCHECK` in both Dockerfiles and a `healthcheck:` in both compose files,
  and records the whole thing in an ADR + CHANGELOG.

**Tech Stack:** Go 1.26.5, `net/http` + `net/http/httptest`, Prometheus
`client_golang`, GoReleaser `dockers_v2`, Docker/Compose, Alpine, MkDocs Material,
`golangci-lint` v2.12.2, `hadolint`.

**Spec:** `/Users/fjacquet/Projects/obs_exporter/docs/superpowers/specs/2026-08-01-family-standard-catch-up-design.md`
(this plan implements its **Plan A** section only; Plans B–E cover `kemp_exporter`,
`nsr_exporter`, `pve_exporter` and `idrac_exporter` and are out of scope here).

---

## Global Constraints

These are non-negotiable and were each learned from a shipped defect. Read them
before touching any file.

1. **`127.0.0.1`, never `localhost`, in every healthcheck.** Alpine's busybox
   `wget` resolves `localhost` via `::1` first, and these exporters bind IPv4
   only. A `localhost`-based healthcheck passes `hadolint` **and**
   `docker compose config` while failing at runtime with connection refused.

2. **The healthcheck timeout is `5s` in BOTH places** — `--timeout=5s` in the
   Dockerfile `HEALTHCHECK` and `timeout: 5s` in the compose `healthcheck:`. A
   prior family-wide effort shipped 5s/10s mismatches across eight repos and had
   to correct every one of them in final review. Same for the other three values:
   `interval: 30s`, `retries: 3`, `start_period: 10s`.

3. **hadolint DL3025 is unavoidable and expected.** The `HEALTHCHECK … || exit 1`
   idiom requires shell-form `CMD`, which DL3025 flags by definition. `DL3007`
   (`:latest` tag) and `DL3066` are standing family findings. All three are
   expected, not defects. **Do NOT add inline `# hadolint ignore=` suppressions**
   and **do NOT treat them as blocking.**

4. **Verify a healthcheck by BUILDING AND RUNNING the image**, then asserting
   `docker inspect --format='{{.State.Health.Status}}' <container>` prints
   `healthy`. Reading the Dockerfile is not verification — the `localhost`/`::1`
   bug passed both hadolint and `docker compose config` and still failed at
   runtime.

5. **Confirm ADR numbers with `ls docs/adr/` before writing the file.** Expected
   next numbers are m365 `0011`, veeam `0002`, vmware `0002` — but confirm, never
   assume. A prior effort shipped literal `ADR-000N` placeholder text into a
   committed Dockerfile comment.

6. **Every new ADR needs a row in `docs/adr/index.md` AND an entry in
   `mkdocs.yml`'s `nav:`.** All three repos list every single ADR explicitly in
   the `nav:` block (m365 `mkdocs.yml:46-62`, veeam `mkdocs.yml:41-48`, vmware
   likewise), so an ADR left out of `nav:` is unreachable from the built site.
   A prior effort missed the index row in one repo.

   Both are discoverability requirements, **not** build gates. Absent a
   `validation:` block — and none of these repos has one — a docs file missing
   from `nav:` is an INFO notice and `mkdocs build --strict` still exits 0
   (verified empirically on a sibling repo carrying two un-navved ADRs).
   `--strict` *does* fail on the reverse: a `nav:` entry pointing at a file that
   does not exist, and broken internal links. So the build will catch a typo'd
   nav path but will never catch a forgotten one — that is on you.

7. **After the change, grep each consumer for `distroless`, `65532` and
   `nonroot`, and fix every USER-FACING hit.** Historical ADRs are records and
   stay as written. Every repo in the prior Alpine effort needed a post-review fix
   wave for exactly this.

8. **Apple Silicon note.** Building `Dockerfile.goreleaser` locally requires an
   arm64 binary laid out where the Dockerfile's `COPY ${TARGETPLATFORM}/…`
   expects it: build with `GOOS=linux GOARCH=arm64` and pass a matching
   `--build-arg TARGETPLATFORM=linux/arm64`. Get this wrong and the container dies
   with `exec format error`, which looks like a healthcheck failure but is not.

9. **`docker-compose.ghcr.yml` runs the *published* image.** Until a release
   carrying the Alpine image is cut, `ghcr.io/fjacquet/<repo>:latest` is still
   distroless and has no `wget` — a runtime healthcheck against it will report
   `unhealthy`. Verify the ghcr compose file with `docker compose -f
   docker-compose.ghcr.yml config -q` **only**; do the runtime `healthy`
   assertion against the locally built image (`docker-compose.yml`).

10. **No inline `//nolint` or `# nosemgrep` suppressions anywhere.** Restructure
    instead.

11. **Commit at the end of each task**, with the message given in that task's
    final step. Do not batch multiple tasks into one commit.

---

## File Structure

| Path | Action |
|---|---|
| `/Users/fjacquet/Projects/licenses-exporter-core/health.go` | Modify — `ServeHTTP` always 200 |
| `/Users/fjacquet/Projects/licenses-exporter-core/health_test.go` | Modify — rewrite for always-200 + `staticOKHandler` |
| `/Users/fjacquet/Projects/licenses-exporter-core/server.go` | Modify — add `staticOKHandler`, register `/livez` + `/readyz` |
| `/Users/fjacquet/Projects/licenses-exporter-core/run_test.go` | Modify — drop the 503 assertion, add a probes test |
| `/Users/fjacquet/Projects/licenses-exporter-core/README.md` | Modify — document the four routes |
| `/Users/fjacquet/Projects/licenses-exporter-core/CHANGELOG.md` | Modify — add `## [Unreleased]`, then `## [1.1.0]` |
| `/Users/fjacquet/Projects/m365_licenses_exporter/go.mod` `go.sum` | Modify — core `v1.0.1` → `v1.1.0` |
| `/Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile` | Modify — add `HEALTHCHECK` (port 9105) |
| `/Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile.goreleaser` | Modify — distroless → Alpine, uid 10001, `HEALTHCHECK` |
| `/Users/fjacquet/Projects/m365_licenses_exporter/docker-compose.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/m365_licenses_exporter/docker-compose.ghcr.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/0011-livez-readyz-probes-and-alpine-release-image.md` | Create |
| `/Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/index.md` | Modify — add row |
| `/Users/fjacquet/Projects/m365_licenses_exporter/mkdocs.yml` | Modify — add nav entry |
| `/Users/fjacquet/Projects/m365_licenses_exporter/docs/deployment/docker.md` | Modify — probes + Alpine release image |
| `/Users/fjacquet/Projects/m365_licenses_exporter/README.md` | Modify — probe URLs |
| `/Users/fjacquet/Projects/m365_licenses_exporter/CHANGELOG.md` | Modify — **create** `## [Unreleased]` (it has none) |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/go.mod` `go.sum` | Modify — core `v1.0.1` → `v1.1.0` |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile` | Modify — add `HEALTHCHECK` (port 9106) |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile.goreleaser` | Modify — distroless → Alpine, uid 10001, `HEALTHCHECK` |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/docker-compose.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/docker-compose.ghcr.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md` | Create |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/index.md` | Modify — add row |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/mkdocs.yml` | Modify — add nav entry |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/deployment/docker.md` | Modify — probes + Alpine release image |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/README.md` | Modify — probe URLs |
| `/Users/fjacquet/Projects/vmware_licenses_exporter/CHANGELOG.md` | Modify — under existing `## [Unreleased]` |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/go.mod` `go.sum` | Modify — core `v1.0.1` → `v1.1.0` |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile` | Modify — add `HEALTHCHECK` (port 9107) |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile.goreleaser` | Modify — distroless → Alpine, uid 10001, `HEALTHCHECK` |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/docker-compose.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/docker-compose.ghcr.yml` | Modify — add `healthcheck:` |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md` | Create |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/index.md` | Modify — add row |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/mkdocs.yml` | Modify — add nav entry |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/deployment/docker.md` | Modify — probes + Alpine release image |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/README.md` | Modify — probe URLs |
| `/Users/fjacquet/Projects/veeam_licenses_exporter/CHANGELOG.md` | Modify — under existing `## [Unreleased]` |

---

# Phase 1 — `licenses-exporter-core` (Tasks 1–5)

All Phase 1 work happens in `/Users/fjacquet/Projects/licenses-exporter-core`.

### Task 1: Failing tests for the always-200 `/health` and the two probes

**Files:**
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/health_test.go`
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/run_test.go`

**Interfaces:**
- Consumes: `Health`, `SetReady()`, `NewServer(Base, version, addr)`, `Server.ln`,
  `Server.RunCollection`, `fakeSource` (from `fake_source_test.go`).
- Produces: `TestHealthAlwaysOKBodyReflectsReadiness`, `TestStaticOKHandler`,
  `TestServerServesProbesBeforeAnyCollection`, and a rewritten
  `TestServerServesMetricsAndHealth`. Referencing `staticOKHandler` (not yet
  defined) makes the package fail to compile — that is the intended red state.

- [x] **Step 1: Replace the whole body of `health_test.go`.** Write this file
  exactly:

```go
package licenses_core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthAlwaysOKBodyReflectsReadiness locks the family probe contract:
// /health answers 200 at ALL times, before and after the first collection
// cycle. Readiness is reported in the body ("starting" -> "ok"), never as the
// status code — a 503 here would make an orchestrator's liveness probe restart
// a process that is merely still doing its first collection.
func TestHealthAlwaysOKBodyReflectsReadiness(t *testing.T) {
	h := &Health{}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pre-ready code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "starting" {
		t.Fatalf("pre-ready body = %q, want %q", got, "starting")
	}

	h.SetReady()

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("post-ready code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("post-ready body = %q, want %q", got, "ok")
	}
}

// TestStaticOKHandler proves the probe handler answers 200 and reads no state.
func TestStaticOKHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("staticOKHandler code = %d, want 200", rec.Code)
	}
}
```

- [x] **Step 2: In `run_test.go`, replace the pre-collection 503 assertion inside
  `TestServerServesMetricsAndHealth`.** Find this block (currently lines 168–175):

```go
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/health before collection = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
```

  and replace it with:

```go
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	preBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read /health body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health before collection = %d, want 200 (never 503)", resp.StatusCode)
	}
	if string(preBody) != "starting" {
		t.Fatalf("/health body before collection = %q, want %q", preBody, "starting")
	}
```

- [x] **Step 3: In `run_test.go`, rewrite the readiness poll loop of the same test
  to poll the BODY, not the status.** Find this block (currently lines 185–199):

```go
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get(base + "/health")
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/health never became ready, last status = %d", resp.StatusCode)
		}
		time.Sleep(5 * time.Millisecond)
	}
```

  and replace it with:

```go
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get(base + "/health")
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		readyBody, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			t.Fatalf("read /health body: %v", rerr)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("/health = %d, want 200 at every point in the lifecycle", resp.StatusCode)
		}
		if string(readyBody) == "ok" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/health never reported ready, last body = %q", readyBody)
		}
		time.Sleep(5 * time.Millisecond)
	}
```

- [x] **Step 4: Append a new server-level probes test to the end of
  `run_test.go`.** Add:

```go
// TestServerServesProbesBeforeAnyCollection proves /livez, /readyz and /health
// all answer 200 on a server that has never run a collection cycle — the
// always-200 contract a Kubernetes livenessProbe / Docker HEALTHCHECK depends
// on. RunCollection is deliberately never called here.
func TestServerServesProbesBeforeAnyCollection(t *testing.T) {
	srv, err := NewServer(Base{}, "v", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	base := "http://" + srv.ln.Addr().String()
	for _, path := range []string{"/livez", "/readyz", "/health"} {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			t.Fatalf("read %s body: %v", path, rerr)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s before any collection = %d, want 200", path, resp.StatusCode)
		}
		if path == "/health" && string(body) != "starting" {
			t.Fatalf("/health body before any collection = %q, want %q", body, "starting")
		}
	}
}
```

- [x] **Step 5: Run the tests and confirm they fail for the right reason.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && go test ./... 2>&1 | head -30
```

  Expect a **compile** failure: `undefined: staticOKHandler`. That is the correct
  red state. If instead you see a passing run, the edits did not land — re-check
  Steps 1–4 before continuing.

- [x] **Step 6: Commit the red tests.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git add health_test.go run_test.go && git commit -m "test: /health always 200 and /livez /readyz probes (red)

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Implement always-200 `/health` and register `/livez` + `/readyz`

**Files:**
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/health.go`
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/server.go`

**Interfaces:**
- Consumes: `net/http`, `sync/atomic`, the existing `Health` struct and `mux`
  in `NewServer`.
- Produces: `staticOKHandler(http.ResponseWriter, *http.Request)` (package-private,
  in `server.go`); routes `/livez` and `/readyz`; `Health.ServeHTTP` that never
  writes a non-200 status. **No exported API changes** — this is why the release
  is a minor, not a major.

- [x] **Step 1: Replace the whole body of `health.go`.** Write this file exactly:

```go
package licenses_core

import (
	"net/http"
	"sync/atomic"
)

// Health always answers 200. The ready flag is reported in the BODY —
// "starting" before the first collection cycle completes, "ok" after — and
// never as the status code: a 503 here makes an orchestrator's liveness probe
// restart a process that is merely still doing its first collection, and makes
// a Docker HEALTHCHECK report the container unhealthy for the whole start-up
// window. Machine-readable liveness/readiness live at /livez and /readyz
// (server.go), which read no state at all; /health stays informational.
type Health struct {
	ready atomic.Bool
}

func (h *Health) SetReady() { h.ready.Store(true) }

func (h *Health) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if h.ready.Load() {
		_, _ = w.Write([]byte("ok"))
		return
	}
	_, _ = w.Write([]byte("starting"))
}
```

- [x] **Step 2: In `server.go`, register the two probe routes.** Find these lines
  (currently 50–53):

```go
	health := &Health{}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.Handle("/health", health)
```

  and replace with:

```go
	health := &Health{}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.Handle("/health", health)
	// /livez and /readyz read no state whatsoever: they answer 200 the moment the
	// listener is bound. Never point a probe at /metrics instead — rendering the
	// whole exposition per probe tick is needless load and can block behind a slow
	// collection cycle.
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
```

- [x] **Step 3: In `server.go`, add the handler itself.** Insert this function
  immediately **above** `func NewServer(` (i.e. after the `Server` struct
  declaration, currently ending at line 30):

```go
// staticOKHandler is the family's trivial probe handler: 200, empty body, no
// state read. Wired to both /livez and /readyz.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
```

- [x] **Step 4: In `server.go`, update the two stale comments that still promise a
  503.** In the `Server` struct doc comment, replace:

```go
// listener. It is built ONCE (NewServer) and reused across every reload — RunCollection swaps only the collection loop, never the store or the
// listener, so /metrics never blanks and /health never flips back to 503 on a
// reload (ADR-0008 §4).
```

  with:

```go
// listener. It is built ONCE (NewServer) and reused across every reload — RunCollection swaps only the collection loop, never the store or the
// listener, so /metrics never blanks and /health never regresses to "starting"
// on a reload (ADR-0008 §4). /health is always 200; readiness is body content.
```

  (Match the existing wrapping of the surrounding lines when you edit — the text
  above is shown unwrapped for clarity; keep the file `gofmt`-clean.) Then in the
  serving goroutine, replace:

```go
		logrus.WithField("addr", addr).Info("serving /metrics and /health")
```

  with:

```go
		logrus.WithField("addr", addr).Info("serving /metrics, /health, /livez and /readyz")
```

  Also update the `NewServer` doc comment's parenthetical
  `(shared store, Prometheus registry+collector, OTLP exporter, /health, one bound listener)`
  to `(shared store, Prometheus registry+collector, OTLP exporter, /health, /livez, /readyz, one bound listener)`.

- [x] **Step 5: Run the tests and confirm green.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && go test ./... 2>&1 | tail -20
```

  Expect `ok  github.com/fjacquet/licenses-exporter-core`. If
  `TestServerServesMetricsAndHealth` still fails, the Task 1 Step 2/3 edits were
  not applied.

- [x] **Step 6: Run the full gate.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && make ci
```

  `lint`, `test` (race + coverage), `build`, `vuln` must all pass.

- [x] **Step 7: Commit.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git add health.go server.go && git commit -m "feat: always-200 /health plus /livez and /readyz probes

/health keeps 'starting'/'ok' as body content and never as the status code, so a
liveness probe cannot restart a process that is merely still doing its first
collection. /livez and /readyz are wired to a handler that reads no state.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Document the routes in core's README

**Files:**
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: consumer-facing documentation of the four HTTP routes.

- [x] **Step 1: Update the lifecycle sentence.** Find line 52, currently:

```
runs the whole lifecycle (`--once`, or serve `/metrics` + `/health` with
```

  and change `/metrics` + `/health` to
  `` `/metrics`, `/health`, `/livez` + `/readyz` `` so the line reads:

```
runs the whole lifecycle (`--once`, or serve `/metrics`, `/health`, `/livez` + `/readyz` with
```

- [x] **Step 2: Add a routes section.** Immediately after the paragraph containing
  line 84 (`collection loop on reload — /metrics never blanks and the socket never rebinds.`),
  insert a blank line and then:

```markdown
### HTTP routes

| Path | Status | Body | Notes |
|---|---|---|---|
| `/metrics` | 200 | Prometheus exposition | Rendered from the current snapshot. |
| `/health` | **always 200** | `starting` until the first collection cycle completes, then `ok` | Informational. The readiness flag is body content, never the status code. |
| `/livez` | 200 | empty | Reads no state; answers as soon as the listener is bound. |
| `/readyz` | 200 | empty | Same handler as `/livez`. |

Point container `HEALTHCHECK`s and Kubernetes probes at `/livez` and `/readyz`,
never at `/metrics`: rendering the full exposition per probe tick is needless
load and can block behind a slow collection cycle.
```

- [x] **Step 3: Verify the file still reads correctly.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && grep -n "livez\|readyz\|always 200" README.md
```

  Expect at least five hits.

- [x] **Step 4: Commit.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git add README.md && git commit -m "docs: document /metrics /health /livez /readyz in the README

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Add a CHANGELOG `## [Unreleased]` heading and the v1.1.0 entry

**Files:**
- Modify: `/Users/fjacquet/Projects/licenses-exporter-core/CHANGELOG.md`

**Interfaces:**
- Consumes: nothing.
- Produces: a Keep-a-Changelog `## [Unreleased]` heading (this file has **none**
  today — its first heading is `## [1.0.0] — 2026-07-02` at line 9) and the
  `## [1.1.0]` entry.

- [x] **Step 1: Insert the two new sections above `## [1.0.0]`.** The file
  currently goes straight from the intro paragraph (ending line 7) to
  `## [1.0.0] — 2026-07-02` on line 9. Insert this block between them, so
  `## [Unreleased]` sits directly under the intro and `## [1.1.0]` directly above
  `## [1.0.0]`:

```markdown
## [Unreleased]

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
```

- [x] **Step 2: Verify the heading order.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && grep -n "^## " CHANGELOG.md
```

  Expect exactly, in order: `## [Unreleased]`, `## [1.1.0] — 2026-08-01`,
  `## [1.0.0] — 2026-07-02`, `## [0.1.0] — 2026-07-02`.

- [x] **Step 3: Commit.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git add CHANGELOG.md && git commit -m "docs(changelog): add [Unreleased] heading and the v1.1.0 entry

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Release `licenses-exporter-core` v1.1.0

**Files:** none — this task tags and publishes.

**Interfaces:**
- Consumes: the commits from Tasks 1–4.
- Produces: the `v1.1.0` tag on `origin`, resolvable by
  `go get github.com/fjacquet/licenses-exporter-core@v1.1.0`. **Every remaining
  task in this plan blocks on this step.**

- [x] **Step 1: Confirm a clean tree and a green gate.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git status --porcelain && make ci
```

  `git status --porcelain` must print nothing.

- [x] **Step 2: Confirm the last released tag, so the new one is genuinely next.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git tag --sort=-v:refname | head -5
```

  Expect `v1.0.1`, `v1.0.0`, `v0.1.0`. If a `v1.1.0` already exists, stop and
  reassess — do not force-move a published tag.

- [x] **Step 3: Push the branch.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && git push origin HEAD
```

- [x] **Step 4: Tag and push the tag.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && \
  git tag -a v1.1.0 -m "v1.1.0: always-200 /health, plus /livez and /readyz probes" && \
  git push origin v1.1.0
```

- [x] **Step 5: Prove the tag is resolvable by the module proxy before any
  consumer tries to bump.**

```bash
cd /tmp && GOFLAGS=-mod=mod go list -m github.com/fjacquet/licenses-exporter-core@v1.1.0
```

  Expect `github.com/fjacquet/licenses-exporter-core v1.1.0`. If the proxy has not
  caught up yet, wait 30 seconds and retry; if it still fails, verify the tag
  exists on the remote with
  `git ls-remote --tags origin | grep v1.1.0` before proceeding.

---

# Phase 2 — `m365_licenses_exporter` (Tasks 6–9)

Port **9105**. All work in `/Users/fjacquet/Projects/m365_licenses_exporter`.
**Blocked on Task 5.**

### Task 6: Bump to core v1.1.0

**Files:**
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/go.mod`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/go.sum`

**Interfaces:**
- Consumes: `github.com/fjacquet/licenses-exporter-core@v1.1.0` (Task 5).
- Produces: an exporter binary whose HTTP server serves `/livez` and `/readyz`
  and whose `/health` is always 200. **No repo code changes** — `main.go`
  delegates to `core.Main` and registers no routes of its own.

- [x] **Step 1: Confirm the current pin is `v1.0.1`.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

  Expect `github.com/fjacquet/licenses-exporter-core v1.0.1`.

- [x] **Step 2: Bump and tidy.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  go get github.com/fjacquet/licenses-exporter-core@v1.1.0 && go mod tidy
```

- [x] **Step 3: Confirm the new pin.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

  Expect `github.com/fjacquet/licenses-exporter-core v1.1.0`.

- [x] **Step 4: Prove the probes are actually served, by running the binary.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && go build -o /tmp/m365_probe_check . && \
  (/tmp/m365_probe_check --config config.yaml --web.listen-address 127.0.0.1:19105 &>/tmp/m365_probe.log &) && \
  sleep 3 && \
  for p in /livez /readyz /health; do printf '%s -> ' "$p"; curl -s -o /tmp/body -w '%{http_code}' "http://127.0.0.1:19105$p"; printf ' %s\n' "$(cat /tmp/body)"; done; \
  pkill -f m365_probe_check
```

  Expect `/livez -> 200`, `/readyz -> 200`, `/health -> 200 starting`. If the
  binary refuses to start because `config.yaml`'s `${M365_*}` refs are unset,
  export throwaway values first
  (`export M365_TENANT_ID=t M365_CLIENT_ID=c M365_CLIENT_SECRET=s`) and retry —
  the probes are the point here, not a successful Graph call.

- [x] **Step 5: Run the gate.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && make ci
```

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && git add go.mod go.sum && git commit -m "feat: bump licenses-exporter-core to v1.1.0 for /livez /readyz

/health is now always 200 with 'starting'/'ok' as body content; /livez and
/readyz are served by the core mux. No repo code changes — main.go delegates
the whole lifecycle to core.Main.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Alpine release image + `HEALTHCHECK` in both Dockerfiles

**Files:**
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile.goreleaser`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile`

**Interfaces:**
- Consumes: `/livez` on port 9105 (Task 6); the GoReleaser `dockers_v2` contract
  — buildx lays the cross-compiled binary out as
  `${TARGETPLATFORM}/m365_licenses_exporter` in the context, and `config.yaml`
  is supplied via `extra_files` (`.goreleaser.yaml:68-75`).
- Produces: a published image on Alpine running as `licenses` uid `10001`
  (**breaking**: was distroless `nonroot`, uid `65532`) with a working
  `HEALTHCHECK`; the same `HEALTHCHECK` in the local build image.

- [x] **Step 1: Replace the whole of `Dockerfile.goreleaser`.** Write exactly:

```dockerfile
# Release image: copies the prebuilt GoReleaser binary (buildx lays it out per-platform
# as ${TARGETPLATFORM}/m365_licenses_exporter in the dockers_v2 context). Alpine, not
# distroless: the family standard is one base image everywhere, and a distroless image
# has no shell and no wget, so it cannot carry the HEALTHCHECK below.
# There is no builder stage to COPY a CA bundle from, so ca-certificates comes from apk.
# For local/dev builds from source, use the multi-stage ./Dockerfile instead.
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 licenses && \
    mkdir -p /var/log/m365_licenses_exporter && \
    chown licenses:licenses /var/log/m365_licenses_exporter

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/m365_licenses_exporter /usr/local/bin/m365_licenses_exporter
COPY config.yaml /etc/m365_licenses_exporter/config.yaml

EXPOSE 9105

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9105/livez || exit 1

USER licenses

ENTRYPOINT ["/usr/local/bin/m365_licenses_exporter"]
CMD ["--config", "/etc/m365_licenses_exporter/config.yaml"]
```

- [x] **Step 2: Add the `HEALTHCHECK` to the local `Dockerfile`.** It currently has
  `EXPOSE 9105` on line 32 and `USER licenses` on line 34. Replace:

```dockerfile
EXPOSE 9105

USER licenses
```

  with:

```dockerfile
EXPOSE 9105

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9105/livez || exit 1

USER licenses
```

- [x] **Step 3: Run hadolint on both files and read, do not act on, the expected
  findings.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  hadolint Dockerfile Dockerfile.goreleaser || true
```

  `DL3025`, `DL3007` and `DL3066` are expected. Add no suppressions. Any *other*
  rule firing is a real finding — fix it.

- [x] **Step 4: Build the local image and assert it reports `healthy`.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  docker build -t m365_licenses_exporter:hc-test . && \
  docker run -d --name m365_hc_test -p 19105:9105 \
    -e M365_TENANT_ID=t -e M365_CLIENT_ID=c -e M365_CLIENT_SECRET=s \
    m365_licenses_exporter:hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' m365_hc_test
```

  **Must print `healthy`.** If it prints `starting`, wait another 30s and re-run
  the `inspect`. If it prints `unhealthy`, inspect the probe output with
  `docker inspect --format='{{json .State.Health.Log}}' m365_hc_test` before
  changing anything — the usual causes are a `localhost` typo or a wrong port.

- [x] **Step 5: Confirm the container runs as uid 10001, not 65532.**

```bash
docker exec m365_hc_test id
```

  Expect `uid=10001(licenses) gid=10001(licenses)`.

- [x] **Step 6: Tear down.**

```bash
docker rm -f m365_hc_test && docker rmi m365_licenses_exporter:hc-test
```

- [x] **Step 7: Build the release image the way GoReleaser will, and assert
  `healthy`.** On Apple Silicon the binary must be arm64 and `TARGETPLATFORM`
  must match, or the container dies with `exec format error`:

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  rm -rf /tmp/m365_grctx && mkdir -p /tmp/m365_grctx/linux/arm64 && \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/m365_grctx/linux/arm64/m365_licenses_exporter . && \
  cp config.yaml /tmp/m365_grctx/config.yaml && \
  cp Dockerfile.goreleaser /tmp/m365_grctx/Dockerfile.goreleaser && \
  docker build -f /tmp/m365_grctx/Dockerfile.goreleaser \
    --build-arg TARGETPLATFORM=linux/arm64 \
    -t m365_licenses_exporter:gr-hc-test /tmp/m365_grctx && \
  docker run -d --name m365_gr_hc_test \
    -e M365_TENANT_ID=t -e M365_CLIENT_ID=c -e M365_CLIENT_SECRET=s \
    m365_licenses_exporter:gr-hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' m365_gr_hc_test
```

  **Must print `healthy`.** On an amd64 host substitute `amd64` for `arm64`
  throughout.

- [x] **Step 8: Tear down and clean up.**

```bash
docker rm -f m365_gr_hc_test && docker rmi m365_licenses_exporter:gr-hc-test && rm -rf /tmp/m365_grctx
```

- [x] **Step 9: Commit.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && git add Dockerfile Dockerfile.goreleaser && git commit -m "feat(docker)!: Alpine release image at uid 10001, HEALTHCHECK on /livez

BREAKING CHANGE: the published image moves from gcr.io/distroless/static:nonroot
(uid 65532) to alpine:latest running as the named user 'licenses' at uid 10001,
matching the local Dockerfile and the rest of the exporter family. Anyone pinning
the container uid — a securityContext runAsUser, a volume's file ownership — must
update it. Both Dockerfiles gain a HEALTHCHECK against /livez on 9105.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: Compose healthchecks

**Files:**
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/docker-compose.yml`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/docker-compose.ghcr.yml`

**Interfaces:**
- Consumes: `/livez` on 9105; the Alpine image from Task 7 (busybox `wget`).
- Produces: a `healthcheck:` on the exporter service in both files, with values
  identical to the Dockerfile's (`interval: 30s`, `timeout: 5s`, `retries: 3`,
  `start_period: 10s`).

- [x] **Step 1: Add the healthcheck to `docker-compose.yml`.** In the
  `m365_licenses_exporter` service, replace:

```yaml
      - M365_CLIENT_SECRET=${M365_CLIENT_SECRET:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - M365_CLIENT_SECRET=${M365_CLIENT_SECRET:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9105/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 2: Add the same healthcheck to `docker-compose.ghcr.yml`.** In the
  `m365_licenses_exporter` service, replace:

```yaml
      - M365_CLIENT_SECRET=${M365_CLIENT_SECRET:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - M365_CLIENT_SECRET=${M365_CLIENT_SECRET:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places. Requires the first
      # release carrying the Alpine image (ADR-0011) or later: every published
      # image before it is distroless and carries no wget.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9105/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 3: Validate both files.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  docker compose -f docker-compose.yml config -q && \
  docker compose -f docker-compose.ghcr.yml config -q && echo "both valid"
```

- [x] **Step 4: Confirm the 5s timeout appears in every one of the four places.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  grep -n "timeout=5s\|timeout: 5s" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml
```

  Expect exactly four hits, one per file.

- [x] **Step 5: Bring the local stack up and assert the exporter is `healthy`.**
  (Do **not** do this for the ghcr stack — it pulls the still-distroless
  published image; see Global Constraint 9.)

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  docker compose up -d m365_licenses_exporter && sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' m365_licenses_exporter
```

  **Must print `healthy`.**

- [x] **Step 6: Tear down.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && docker compose down
```

- [x] **Step 7: Commit.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && git add docker-compose.yml docker-compose.ghcr.yml && git commit -m "feat(compose): healthcheck the exporter against /livez on 9105

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: ADR, index row, nav entry, CHANGELOG, docs sweep

**Files:**
- Create: `/Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/0011-livez-readyz-probes-and-alpine-release-image.md`
  *(confirm `0011` is next in Step 1)*
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/index.md`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/mkdocs.yml`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/CHANGELOG.md`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/docs/deployment/docker.md`
- Modify: `/Users/fjacquet/Projects/m365_licenses_exporter/README.md`

**Interfaces:**
- Consumes: Tasks 6–8.
- Produces: a numbered ADR, a row in the ADR index, an `mkdocs.yml` nav entry, a
  `## [Unreleased]` CHANGELOG section (**this repo has none — create it**), and
  user-facing docs that no longer claim 503-on-startup or a distroless image.

- [x] **Step 1: Confirm the next ADR number. Never assume.**

```bash
ls /Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/
```

  Expect `0001`…`0010` plus `index.md`, making **`0011`** next. If the highest is
  different, use *that* number + 1 in every filename and reference below.

- [x] **Step 2: Create the ADR** at
  `docs/adr/0011-livez-readyz-probes-and-alpine-release-image.md`:

```markdown
# 11. `/livez` + `/readyz` probes and an Alpine release image

Date: 2026-08-01

## Status
Accepted

## Context
Two family-wide standards completed on 2026-08-01 — the always-200 `/livez` +
`/readyz` probe pattern, and the Alpine container-image standard — both skipped
this repo, because the `exporter-standards` skill's family table listed eight
repos and this one was not among them.

Concretely, two defects followed. First, `/health` (served by
`licenses-exporter-core`) returned **503 `starting`** until the first collection
cycle completed. With a default `collection.interval` and a slow Graph tenant that
window is long enough for a Kubernetes `livenessProbe` to restart a perfectly
healthy process, and long enough for a Docker `HEALTHCHECK` to report the
container unhealthy throughout start-up. Second, the published image was
`gcr.io/distroless/static:nonroot`, which has no shell and no `wget` — so the
image *could not carry a `HEALTHCHECK` at all*, while the local `./Dockerfile`
was already Alpine at uid 10001. The two build paths disagreed about the runtime.

All three licenses exporters share their HTTP wiring through
`licenses-exporter-core`: it owns the only `http.ServeMux` and the only `/health`
handler, and no consumer registers a route. So the probe half is one library
change plus three dependency bumps.

## Decision
Consume `licenses-exporter-core` **v1.1.0**, which serves `/livez` and `/readyz`
from a handler that reads no state and makes `/health` answer 200
unconditionally, keeping `starting`/`ok` as body content.

Convert `Dockerfile.goreleaser` from `gcr.io/distroless/static:nonroot` to
`alpine:latest`, running as the named user `licenses` at uid **10001** — matching
the local `./Dockerfile` and the rest of the exporter family. Add a `HEALTHCHECK`
against `http://127.0.0.1:9105/livez` to both Dockerfiles and a matching
`healthcheck:` to both compose files.

`127.0.0.1` and never `localhost`: busybox `wget` resolves `localhost` via `::1`
first and this exporter binds IPv4 only, so a `localhost` check fails at runtime
while passing both `hadolint` and `docker compose config`.

Probes never point at `/metrics`: rendering the full exposition per probe tick is
needless load and can block behind a slow collection cycle.

## Consequences
- **Breaking:** the published container's uid moves `65532` → `10001`. Anyone
  pinning it — a `securityContext.runAsUser`, ownership on a mounted secret or
  log volume — must update. This repo ships no Helm chart, so there is no chart
  default to change.
- The release image gains a shell and busybox. That is a deliberate trade: a
  working `HEALTHCHECK` and one base image across fifteen repos, bought with a
  larger attack surface than distroless.
- `alpine:latest` is unpinned, family-wide. It is the one build input whose
  contents can change between two builds of the same commit, which cuts against
  ADR-0001's supply-chain posture. Uniformity was chosen over reproducibility;
  revisiting it is a fifteen-repo decision, not a per-repo one.
- Anything asserting a 503 from `/health` — an alert rule, a smoke test, a
  blackbox-exporter check — must move to reading the body, or to `/readyz`.
- Engine behaviour arrives by a core version bump, not a local edit (ADR-0010).
```

- [x] **Step 3: Add the index row.** In `docs/adr/index.md`, after the `0010` row
  (line 21), insert:

```markdown
| [0011](0011-livez-readyz-probes-and-alpine-release-image.md) | `/livez` + `/readyz` probes and an Alpine release image at uid 10001 | accepted |
```

  and update the closing sentence so it points at the newest record: replace

```markdown
To add a decision, copy [`0010`](0010-consume-licenses-exporter-core.md)'s structure to
the next number and link it here.
```

  with

```markdown
To add a decision, copy [`0011`](0011-livez-readyz-probes-and-alpine-release-image.md)'s
structure to the next number and link it here.
```

- [x] **Step 4: Add the mkdocs nav entry.** In `mkdocs.yml`, after the `0010` line
  (line 62), insert:

```yaml
      - 0011 /livez + /readyz probes, Alpine release image: adr/0011-livez-readyz-probes-and-alpine-release-image.md
```

- [x] **Step 5: Create the CHANGELOG `## [Unreleased]` section.** This file has
  **no** `## [Unreleased]` heading — its first heading is `## [1.1.2] — 2026-07-10`
  on line 7. Insert this block between the intro paragraph (ending line 5) and
  that heading:

```markdown
## [Unreleased]

### Breaking
- The published container image runs as uid **10001** (named user `licenses`),
  not `65532`. `Dockerfile.goreleaser` moves from `gcr.io/distroless/static:nonroot`
  to `alpine:latest`, matching the local `./Dockerfile` and the rest of the
  exporter family. Anyone pinning the container uid — a `securityContext.runAsUser`,
  ownership on a mounted secret or log volume — must update it. See ADR-0011.

### Added
- **`/livez` and `/readyz`**, both always 200 and reading no state, via
  `licenses-exporter-core` v1.1.0. Point Kubernetes probes and container
  healthchecks at these, never at `/metrics`.
- **`HEALTHCHECK`** against `http://127.0.0.1:9105/livez` in both `Dockerfile` and
  `Dockerfile.goreleaser`, and a matching `healthcheck:` in `docker-compose.yml`
  and `docker-compose.ghcr.yml`. The ghcr healthcheck needs an image from this
  release or later — earlier published images are distroless and carry no `wget`.

### Changed
- **`/health` now always returns 200**, with `starting`/`ok` as the body rather
  than the status code. It previously returned 503 until the first collection
  cycle completed, which restarted healthy pods under a `livenessProbe` and
  reported containers unhealthy for the whole start-up window. Anything asserting
  a 503 from `/health` must be updated.
```

- [x] **Step 6: Update `docs/deployment/docker.md`.** Replace lines 25–26:

```markdown
`/metrics` and `/health` are both served on `9105`; `/health` returns HTTP 200 with
`starting` until the first collection cycle completes for every enabled source, then `ok`.
```

  with:

```markdown
Four routes are served on `9105`:

| Path | Status | Body |
|---|---|---|
| `/metrics` | 200 | Prometheus exposition |
| `/health` | always 200 | `starting` until the first collection cycle completes for every enabled source, then `ok` |
| `/livez` | always 200 | empty |
| `/readyz` | always 200 | empty |

Point Kubernetes probes and container healthchecks at `/livez` and `/readyz` —
never at `/metrics`, which renders the whole exposition per probe tick and can
block behind a slow collection cycle. Both Dockerfiles ship a `HEALTHCHECK`
against `http://127.0.0.1:9105/livez` (`127.0.0.1`, not `localhost`: busybox
`wget` tries `::1` first and the exporter binds IPv4 only), and both compose
files carry the matching `healthcheck:`.
```

  Then update the opening paragraph (lines 3–5) so it covers *both* images —
  replace:

```markdown
The image (`Dockerfile`) is a non-root, multi-stage Alpine build: it runs as the
unprivileged `licenses` user (uid `10001`), listens on `9105`, and reads `config.yaml` from
`/etc/m365_licenses_exporter/config.yaml`.
```

  with:

```markdown
Both images are non-root Alpine builds running as the unprivileged `licenses`
user (uid `10001`), listening on `9105` and reading `config.yaml` from
`/etc/m365_licenses_exporter/config.yaml`. `Dockerfile` is the multi-stage build
from source; `Dockerfile.goreleaser` is the release image published to GHCR,
which copies the prebuilt binary. Published images before the ADR-0011 release
were `gcr.io/distroless/static:nonroot` at uid `65532`.
```

- [x] **Step 7: Update `README.md`.** Replace line 40:

```markdown
# metrics: http://localhost:9105/metrics   health: http://localhost:9105/health
```

  with:

```markdown
# metrics: http://localhost:9105/metrics   health: http://localhost:9105/health
# probes:  http://localhost:9105/livez     http://localhost:9105/readyz  (always 200)
```

- [x] **Step 8: Sweep for falsified claims.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  grep -rn "distroless\|65532\|nonroot" \
    --include="*.md" --include="*.yml" --include="*.yaml" --include="Dockerfile*" --include="*.go" . \
  | grep -v "^./site/"
```

  The only acceptable remaining hits are inside `docs/adr/` (historical records
  and ADR-0011's own text), `CHANGELOG.md`, and the `docs/deployment/docker.md`
  sentence added in Step 6 — all of which describe the *past* state explicitly.
  Any other hit is a stale user-facing claim: fix it.

- [x] **Step 9: Build the docs site strictly.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict
```

  Must exit 0. A missing nav entry or a broken ADR link fails here.

- [x] **Step 10: Run the gate one more time.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && make ci
```

- [x] **Step 11: Commit.**

```bash
cd /Users/fjacquet/Projects/m365_licenses_exporter && \
  git add docs/adr CHANGELOG.md mkdocs.yml docs/deployment/docker.md README.md && \
  git commit -m "docs: ADR-0011 for the probes + Alpine release image, changelog, doc sweep

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

# Phase 3 — `vmware_licenses_exporter` (Tasks 10–13)

Port **9106**. All work in `/Users/fjacquet/Projects/vmware_licenses_exporter`.
**Blocked on Task 5.**

### Task 10: Bump to core v1.1.0

**Files:**
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/go.mod`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/go.sum`

**Interfaces:**
- Consumes: `github.com/fjacquet/licenses-exporter-core@v1.1.0` (Task 5).
- Produces: an exporter serving `/livez` and `/readyz` with an always-200
  `/health`. **No repo code changes** — `main.go` delegates to `core.Main`.

- [x] **Step 1: Confirm the current pin is `v1.0.1`.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

- [x] **Step 2: Bump and tidy.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  go get github.com/fjacquet/licenses-exporter-core@v1.1.0 && go mod tidy
```

- [x] **Step 3: Confirm the new pin.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

  Expect `github.com/fjacquet/licenses-exporter-core v1.1.0`.

- [x] **Step 4: Prove the probes are actually served, by running the binary.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && go build -o /tmp/vmware_probe_check . && \
  (VC_HOST=https://vcenter.invalid VC_USERNAME=u VC_PASSWORD=p \
   /tmp/vmware_probe_check --config config.yaml --web.listen-address 127.0.0.1:19106 &>/tmp/vmware_probe.log &) && \
  sleep 3 && \
  for p in /livez /readyz /health; do printf '%s -> ' "$p"; curl -s -o /tmp/body -w '%{http_code}' "http://127.0.0.1:19106$p"; printf ' %s\n' "$(cat /tmp/body)"; done; \
  pkill -f vmware_probe_check
```

  Expect `/livez -> 200`, `/readyz -> 200`, `/health -> 200 starting`. The vCenter
  itself being unreachable is fine — the probes are the point.

- [x] **Step 5: Run the gate.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && make ci
```

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && git add go.mod go.sum && git commit -m "feat: bump licenses-exporter-core to v1.1.0 for /livez /readyz

/health is now always 200 with 'starting'/'ok' as body content; /livez and
/readyz are served by the core mux. No repo code changes — main.go delegates
the whole lifecycle to core.Main.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 11: Alpine release image + `HEALTHCHECK` in both Dockerfiles

**Files:**
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile.goreleaser`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile`

**Interfaces:**
- Consumes: `/livez` on port 9106 (Task 10); the GoReleaser `dockers_v2` layout
  `${TARGETPLATFORM}/vmware_licenses_exporter` with `config.yaml` in
  `extra_files`.
- Produces: an Alpine release image running as `licenses` uid `10001`
  (**breaking**: was distroless `nonroot`, uid `65532`) with a working
  `HEALTHCHECK`; the same `HEALTHCHECK` in the local build image.

- [x] **Step 1: Replace the whole of `Dockerfile.goreleaser`.** Write exactly:

```dockerfile
# Release image: copies the prebuilt GoReleaser binary (buildx lays it out per-platform
# as ${TARGETPLATFORM}/vmware_licenses_exporter in the dockers_v2 context). Alpine, not
# distroless: the family standard is one base image everywhere, and a distroless image
# has no shell and no wget, so it cannot carry the HEALTHCHECK below.
# There is no builder stage to COPY a CA bundle from, so ca-certificates comes from apk.
# For local/dev builds from source, use the multi-stage ./Dockerfile instead.
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 licenses && \
    mkdir -p /var/log/vmware_licenses_exporter && \
    chown licenses:licenses /var/log/vmware_licenses_exporter

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/vmware_licenses_exporter /usr/local/bin/vmware_licenses_exporter
COPY config.yaml /etc/vmware_licenses_exporter/config.yaml

EXPOSE 9106

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9106/livez || exit 1

USER licenses

ENTRYPOINT ["/usr/local/bin/vmware_licenses_exporter"]
CMD ["--config", "/etc/vmware_licenses_exporter/config.yaml"]
```

- [x] **Step 2: Add the `HEALTHCHECK` to the local `Dockerfile`.** It currently has
  `EXPOSE 9106` on line 32 and `USER licenses` on line 34. Replace:

```dockerfile
EXPOSE 9106

USER licenses
```

  with:

```dockerfile
EXPOSE 9106

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9106/livez || exit 1

USER licenses
```

- [x] **Step 3: Run hadolint on both files.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  hadolint Dockerfile Dockerfile.goreleaser || true
```

  `DL3025`, `DL3007`, `DL3066` are expected. Add no suppressions. Any other rule
  firing is a real finding — fix it.

- [x] **Step 4: Build the local image and assert it reports `healthy`.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  docker build -t vmware_licenses_exporter:hc-test . && \
  docker run -d --name vmware_hc_test -p 19106:9106 \
    -e VC_HOST=https://vcenter.invalid -e VC_USERNAME=u -e VC_PASSWORD=p \
    vmware_licenses_exporter:hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' vmware_hc_test
```

  **Must print `healthy`.** If `unhealthy`, read
  `docker inspect --format='{{json .State.Health.Log}}' vmware_hc_test` before
  changing anything.

- [x] **Step 5: Confirm the container runs as uid 10001.**

```bash
docker exec vmware_hc_test id
```

  Expect `uid=10001(licenses) gid=10001(licenses)`.

- [x] **Step 6: Tear down.**

```bash
docker rm -f vmware_hc_test && docker rmi vmware_licenses_exporter:hc-test
```

- [x] **Step 7: Build the release image the way GoReleaser will, and assert
  `healthy`.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  rm -rf /tmp/vmware_grctx && mkdir -p /tmp/vmware_grctx/linux/arm64 && \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/vmware_grctx/linux/arm64/vmware_licenses_exporter . && \
  cp config.yaml /tmp/vmware_grctx/config.yaml && \
  cp Dockerfile.goreleaser /tmp/vmware_grctx/Dockerfile.goreleaser && \
  docker build -f /tmp/vmware_grctx/Dockerfile.goreleaser \
    --build-arg TARGETPLATFORM=linux/arm64 \
    -t vmware_licenses_exporter:gr-hc-test /tmp/vmware_grctx && \
  docker run -d --name vmware_gr_hc_test \
    -e VC_HOST=https://vcenter.invalid -e VC_USERNAME=u -e VC_PASSWORD=p \
    vmware_licenses_exporter:gr-hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' vmware_gr_hc_test
```

  **Must print `healthy`.** On an amd64 host substitute `amd64` for `arm64`
  throughout; a mismatch produces `exec format error`, not a probe failure.

- [x] **Step 8: Tear down and clean up.**

```bash
docker rm -f vmware_gr_hc_test && docker rmi vmware_licenses_exporter:gr-hc-test && rm -rf /tmp/vmware_grctx
```

- [x] **Step 9: Commit.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && git add Dockerfile Dockerfile.goreleaser && git commit -m "feat(docker)!: Alpine release image at uid 10001, HEALTHCHECK on /livez

BREAKING CHANGE: the published image moves from gcr.io/distroless/static:nonroot
(uid 65532) to alpine:latest running as the named user 'licenses' at uid 10001,
matching the local Dockerfile and the rest of the exporter family. Anyone pinning
the container uid — a securityContext runAsUser, a volume's file ownership — must
update it. Both Dockerfiles gain a HEALTHCHECK against /livez on 9106.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 12: Compose healthchecks

**Files:**
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/docker-compose.yml`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/docker-compose.ghcr.yml`

**Interfaces:**
- Consumes: `/livez` on 9106; the Alpine image from Task 11.
- Produces: a `healthcheck:` on the exporter service in both files, with values
  identical to the Dockerfile's.

- [x] **Step 1: Add the healthcheck to `docker-compose.yml`.** In the
  `vmware_licenses_exporter` service, replace:

```yaml
      - VC_PASSWORD=${VC_PASSWORD:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - VC_PASSWORD=${VC_PASSWORD:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9106/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 2: Add the same healthcheck to `docker-compose.ghcr.yml`.** In the
  `vmware_licenses_exporter` service, replace:

```yaml
      - VC_PASSWORD=${VC_PASSWORD:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - VC_PASSWORD=${VC_PASSWORD:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places. Requires the first
      # release carrying the Alpine image (ADR-0002) or later: every published
      # image before it is distroless and carries no wget.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9106/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 3: Validate both files.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  docker compose -f docker-compose.yml config -q && \
  docker compose -f docker-compose.ghcr.yml config -q && echo "both valid"
```

- [x] **Step 4: Confirm the 5s timeout appears in every one of the four places.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  grep -n "timeout=5s\|timeout: 5s" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml
```

  Expect exactly four hits, one per file.

- [x] **Step 5: Bring the local stack up and assert the exporter is `healthy`.**
  Not the ghcr stack — see Global Constraint 9.

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  docker compose up -d vmware_licenses_exporter && sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' vmware_licenses_exporter
```

  **Must print `healthy`.**

- [x] **Step 6: Tear down.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && docker compose down
```

- [x] **Step 7: Commit.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && git add docker-compose.yml docker-compose.ghcr.yml && git commit -m "feat(compose): healthcheck the exporter against /livez on 9106

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 13: ADR, index row, nav entry, CHANGELOG, docs sweep

**Files:**
- Create: `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md`
  *(confirm `0002` is next in Step 1)*
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/index.md`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/mkdocs.yml`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/CHANGELOG.md`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/docs/deployment/docker.md`
- Modify: `/Users/fjacquet/Projects/vmware_licenses_exporter/README.md`

**Interfaces:**
- Consumes: Tasks 10–12.
- Produces: a numbered ADR, an index row, an `mkdocs.yml` nav entry, CHANGELOG
  entries under the **existing** `## [Unreleased]` heading (line 7), and docs that
  no longer claim 503-on-startup or a distroless image.

- [x] **Step 1: Confirm the next ADR number. Never assume.**

```bash
ls /Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/
```

  Expect `0001-consume-core-retain-govmomi.md` and `index.md`, making **`0002`**
  next. If the highest is different, use *that* number + 1 throughout.

- [x] **Step 2: Create the ADR** at
  `docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md`:

```markdown
# 2. `/livez` + `/readyz` probes and an Alpine release image

Date: 2026-08-01

## Status
Accepted

## Context
Two family-wide standards completed on 2026-08-01 — the always-200 `/livez` +
`/readyz` probe pattern, and the Alpine container-image standard — both skipped
this repo, because the `exporter-standards` skill's family table listed eight
repos and this one was not among them.

Concretely, two defects followed. First, `/health` (served by
`licenses-exporter-core`) returned **503 `starting`** until the first collection
cycle completed. Against a slow or unreachable vCenter that window is long enough
for a Kubernetes `livenessProbe` to restart a perfectly healthy process, and long
enough for a Docker `HEALTHCHECK` to report the container unhealthy throughout
start-up. Second, the published image was `gcr.io/distroless/static:nonroot`,
which has no shell and no `wget` — so the image *could not carry a `HEALTHCHECK`
at all*, while the local `./Dockerfile` was already Alpine at uid 10001. The two
build paths disagreed about the runtime.

All three licenses exporters share their HTTP wiring through
`licenses-exporter-core`: it owns the only `http.ServeMux` and the only `/health`
handler, and no consumer registers a route. So the probe half is one library
change plus three dependency bumps.

## Decision
Consume `licenses-exporter-core` **v1.1.0**, which serves `/livez` and `/readyz`
from a handler that reads no state and makes `/health` answer 200
unconditionally, keeping `starting`/`ok` as body content.

Convert `Dockerfile.goreleaser` from `gcr.io/distroless/static:nonroot` to
`alpine:latest`, running as the named user `licenses` at uid **10001** — matching
the local `./Dockerfile` and the rest of the exporter family. Add a `HEALTHCHECK`
against `http://127.0.0.1:9106/livez` to both Dockerfiles and a matching
`healthcheck:` to both compose files.

`127.0.0.1` and never `localhost`: busybox `wget` resolves `localhost` via `::1`
first and this exporter binds IPv4 only, so a `localhost` check fails at runtime
while passing both `hadolint` and `docker compose config`.

Probes never point at `/metrics`: rendering the full exposition per probe tick is
needless load and can block behind a slow collection cycle.

## Consequences
- **Breaking:** the published container's uid moves `65532` → `10001`. Anyone
  pinning it — a `securityContext.runAsUser`, ownership on a mounted secret or
  log volume — must update. This repo ships no Helm chart, so there is no chart
  default to change.
- The release image gains a shell and busybox. That is a deliberate trade: a
  working `HEALTHCHECK` and one base image across fifteen repos, bought with a
  larger attack surface than distroless.
- `alpine:latest` is unpinned, family-wide. It is the one build input whose
  contents can change between two builds of the same commit. Uniformity was
  chosen over reproducibility; revisiting it is a fifteen-repo decision, not a
  per-repo one.
- Anything asserting a 503 from `/health` — an alert rule, a smoke test, a
  blackbox-exporter check — must move to reading the body, or to `/readyz`.
- Engine behaviour arrives by a core version bump, not a local edit (ADR-0001).
```

- [x] **Step 3: Add the index row.** In `docs/adr/index.md`, after the `0001` row
  (line 10), insert:

```markdown
| [0002](0002-livez-readyz-probes-and-alpine-release-image.md) | `/livez` + `/readyz` probes and an Alpine release image at uid 10001 | accepted |
```

  and update the closing sentence — replace:

```markdown
To add a decision, copy [`0001`](0001-consume-core-retain-govmomi.md)'s structure to
the next number and link it here.
```

  with:

```markdown
To add a decision, copy [`0002`](0002-livez-readyz-probes-and-alpine-release-image.md)'s
structure to the next number and link it here.
```

- [x] **Step 4: Add the mkdocs nav entry.** In `mkdocs.yml`, after the `0001` line
  in the `Architecture Decisions:` block, insert:

```yaml
      - 0002 /livez + /readyz probes, Alpine release image: adr/0002-livez-readyz-probes-and-alpine-release-image.md
```

- [x] **Step 5: Fill in the existing `## [Unreleased]` CHANGELOG section.** This
  file already has `## [Unreleased]` on line 7, immediately followed by
  `## [1.0.2] - 2026-07-10` on line 9. Insert this content between them:

```markdown
### Breaking
- The published container image runs as uid **10001** (named user `licenses`),
  not `65532`. `Dockerfile.goreleaser` moves from `gcr.io/distroless/static:nonroot`
  to `alpine:latest`, matching the local `./Dockerfile` and the rest of the
  exporter family. Anyone pinning the container uid — a `securityContext.runAsUser`,
  ownership on a mounted secret or log volume — must update it. See ADR-0002.

### Added
- **`/livez` and `/readyz`**, both always 200 and reading no state, via
  `licenses-exporter-core` v1.1.0. Point Kubernetes probes and container
  healthchecks at these, never at `/metrics`.
- **`HEALTHCHECK`** against `http://127.0.0.1:9106/livez` in both `Dockerfile` and
  `Dockerfile.goreleaser`, and a matching `healthcheck:` in `docker-compose.yml`
  and `docker-compose.ghcr.yml`. The ghcr healthcheck needs an image from this
  release or later — earlier published images are distroless and carry no `wget`.

### Changed
- **`/health` now always returns 200**, with `starting`/`ok` as the body rather
  than the status code. It previously returned 503 until the first collection
  cycle completed, which restarted healthy pods under a `livenessProbe` and
  reported containers unhealthy for the whole start-up window. Anything asserting
  a 503 from `/health` must be updated.
```

- [x] **Step 6: Update `docs/deployment/docker.md`.** Replace lines 25–26:

```markdown
`/metrics` and `/health` are both served on `9106`; `/health` returns HTTP 200 with
`starting` until the first collection cycle completes for every enabled vCenter, then `ok`.
```

  with:

```markdown
Four routes are served on `9106`:

| Path | Status | Body |
|---|---|---|
| `/metrics` | 200 | Prometheus exposition |
| `/health` | always 200 | `starting` until the first collection cycle completes for every enabled vCenter, then `ok` |
| `/livez` | always 200 | empty |
| `/readyz` | always 200 | empty |

Point Kubernetes probes and container healthchecks at `/livez` and `/readyz` —
never at `/metrics`, which renders the whole exposition per probe tick and can
block behind a slow collection cycle. Both Dockerfiles ship a `HEALTHCHECK`
against `http://127.0.0.1:9106/livez` (`127.0.0.1`, not `localhost`: busybox
`wget` tries `::1` first and the exporter binds IPv4 only), and both compose
files carry the matching `healthcheck:`.
```

  Then check the file's opening paragraph (lines 1–5) with
  `sed -n '1,10p' docs/deployment/docker.md`: if it describes only the local
  `Dockerfile`, extend it to cover both images, as:

```markdown
Both images are non-root Alpine builds running as the unprivileged `licenses`
user (uid `10001`), listening on `9106` and reading `config.yaml` from
`/etc/vmware_licenses_exporter/config.yaml`. `Dockerfile` is the multi-stage
build from source; `Dockerfile.goreleaser` is the release image published to
GHCR, which copies the prebuilt binary. Published images before the ADR-0002
release were `gcr.io/distroless/static:nonroot` at uid `65532`.
```

- [x] **Step 7: Update `README.md`.** Replace line 41:

```markdown
# metrics: http://localhost:9106/metrics   health: http://localhost:9106/health
```

  with:

```markdown
# metrics: http://localhost:9106/metrics   health: http://localhost:9106/health
# probes:  http://localhost:9106/livez     http://localhost:9106/readyz  (always 200)
```

- [x] **Step 8: Sweep for falsified claims.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  grep -rn "distroless\|65532\|nonroot" \
    --include="*.md" --include="*.yml" --include="*.yaml" --include="Dockerfile*" --include="*.go" . \
  | grep -v "^./site/"
```

  The only acceptable remaining hits are inside `docs/adr/`, `CHANGELOG.md`, and
  the `docs/deployment/docker.md` sentence added in Step 6 — all of which describe
  the *past* state explicitly. Any other hit is a stale user-facing claim: fix it.

- [x] **Step 9: Build the docs site strictly.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict
```

  Must exit 0.

- [x] **Step 10: Run the gate one more time.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && make ci
```

- [x] **Step 11: Commit.**

```bash
cd /Users/fjacquet/Projects/vmware_licenses_exporter && \
  git add docs/adr CHANGELOG.md mkdocs.yml docs/deployment/docker.md README.md && \
  git commit -m "docs: ADR-0002 for the probes + Alpine release image, changelog, doc sweep

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

# Phase 4 — `veeam_licenses_exporter` (Tasks 14–17)

Port **9107**. All work in `/Users/fjacquet/Projects/veeam_licenses_exporter`.
**Blocked on Task 5.**

### Task 14: Bump to core v1.1.0

**Files:**
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/go.mod`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/go.sum`

**Interfaces:**
- Consumes: `github.com/fjacquet/licenses-exporter-core@v1.1.0` (Task 5).
- Produces: an exporter serving `/livez` and `/readyz` with an always-200
  `/health`. **No repo code changes** — `main.go` delegates to `core.Main`.

- [x] **Step 1: Confirm the current pin is `v1.0.1`.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

- [x] **Step 2: Bump and tidy.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  go get github.com/fjacquet/licenses-exporter-core@v1.1.0 && go mod tidy
```

- [x] **Step 3: Confirm the new pin.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && grep -n "licenses-exporter-core" go.mod
```

  Expect `github.com/fjacquet/licenses-exporter-core v1.1.0`.

- [x] **Step 4: Prove the probes are actually served, by running the binary.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && go build -o /tmp/veeam_probe_check . && \
  (VEEAM_EM_HOST=https://em.invalid:9398 VEEAM_USERNAME=u VEEAM_PASSWORD=p \
   /tmp/veeam_probe_check --config config.yaml --web.listen-address 127.0.0.1:19107 &>/tmp/veeam_probe.log &) && \
  sleep 3 && \
  for p in /livez /readyz /health; do printf '%s -> ' "$p"; curl -s -o /tmp/body -w '%{http_code}' "http://127.0.0.1:19107$p"; printf ' %s\n' "$(cat /tmp/body)"; done; \
  pkill -f veeam_probe_check
```

  Expect `/livez -> 200`, `/readyz -> 200`, `/health -> 200 starting`. Enterprise
  Manager being unreachable is fine — the probes are the point.

- [x] **Step 5: Run the gate.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && make ci
```

- [x] **Step 6: Commit.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && git add go.mod go.sum && git commit -m "feat: bump licenses-exporter-core to v1.1.0 for /livez /readyz

/health is now always 200 with 'starting'/'ok' as body content; /livez and
/readyz are served by the core mux. No repo code changes — main.go delegates
the whole lifecycle to core.Main.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 15: Alpine release image + `HEALTHCHECK` in both Dockerfiles

**Files:**
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile.goreleaser`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile`

**Interfaces:**
- Consumes: `/livez` on port 9107 (Task 14); the GoReleaser `dockers_v2` layout
  `${TARGETPLATFORM}/veeam_licenses_exporter` with `config.yaml` in `extra_files`.
- Produces: an Alpine release image running as `licenses` uid `10001`
  (**breaking**: was distroless `nonroot`, uid `65532`) with a working
  `HEALTHCHECK`; the same `HEALTHCHECK` in the local build image.

- [x] **Step 1: Replace the whole of `Dockerfile.goreleaser`.** Write exactly:

```dockerfile
# Release image: copies the prebuilt GoReleaser binary (buildx lays it out per-platform
# as ${TARGETPLATFORM}/veeam_licenses_exporter in the dockers_v2 context). Alpine, not
# distroless: the family standard is one base image everywhere, and a distroless image
# has no shell and no wget, so it cannot carry the HEALTHCHECK below.
# There is no builder stage to COPY a CA bundle from, so ca-certificates comes from apk.
# For local/dev builds from source, use the multi-stage ./Dockerfile instead.
FROM alpine:latest

RUN apk --no-cache add ca-certificates && \
    adduser -D -u 10001 licenses && \
    mkdir -p /var/log/veeam_licenses_exporter && \
    chown licenses:licenses /var/log/veeam_licenses_exporter

ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/veeam_licenses_exporter /usr/local/bin/veeam_licenses_exporter
COPY config.yaml /etc/veeam_licenses_exporter/config.yaml

EXPOSE 9107

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9107/livez || exit 1

USER licenses

ENTRYPOINT ["/usr/local/bin/veeam_licenses_exporter"]
CMD ["--config", "/etc/veeam_licenses_exporter/config.yaml"]
```

- [x] **Step 2: Add the `HEALTHCHECK` to the local `Dockerfile`.** It currently has
  `EXPOSE 9107` on line 32 and `USER licenses` on line 34. Replace:

```dockerfile
EXPOSE 9107

USER licenses
```

  with:

```dockerfile
EXPOSE 9107

# 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first and the
# exporter binds IPv4 only. The `|| exit 1` idiom requires shell-form CMD, so hadolint
# DL3025 fires here by construction — expected family-wide, not a defect.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://127.0.0.1:9107/livez || exit 1

USER licenses
```

- [x] **Step 3: Run hadolint on both files.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  hadolint Dockerfile Dockerfile.goreleaser || true
```

  `DL3025`, `DL3007`, `DL3066` are expected. Add no suppressions. Any other rule
  firing is a real finding — fix it.

- [x] **Step 4: Build the local image and assert it reports `healthy`.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  docker build -t veeam_licenses_exporter:hc-test . && \
  docker run -d --name veeam_hc_test -p 19107:9107 \
    -e VEEAM_EM_HOST=https://em.invalid:9398 -e VEEAM_USERNAME=u -e VEEAM_PASSWORD=p \
    veeam_licenses_exporter:hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' veeam_hc_test
```

  **Must print `healthy`.** If `unhealthy`, read
  `docker inspect --format='{{json .State.Health.Log}}' veeam_hc_test` before
  changing anything.

- [x] **Step 5: Confirm the container runs as uid 10001.**

```bash
docker exec veeam_hc_test id
```

  Expect `uid=10001(licenses) gid=10001(licenses)`.

- [x] **Step 6: Tear down.**

```bash
docker rm -f veeam_hc_test && docker rmi veeam_licenses_exporter:hc-test
```

- [x] **Step 7: Build the release image the way GoReleaser will, and assert
  `healthy`.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  rm -rf /tmp/veeam_grctx && mkdir -p /tmp/veeam_grctx/linux/arm64 && \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/veeam_grctx/linux/arm64/veeam_licenses_exporter . && \
  cp config.yaml /tmp/veeam_grctx/config.yaml && \
  cp Dockerfile.goreleaser /tmp/veeam_grctx/Dockerfile.goreleaser && \
  docker build -f /tmp/veeam_grctx/Dockerfile.goreleaser \
    --build-arg TARGETPLATFORM=linux/arm64 \
    -t veeam_licenses_exporter:gr-hc-test /tmp/veeam_grctx && \
  docker run -d --name veeam_gr_hc_test \
    -e VEEAM_EM_HOST=https://em.invalid:9398 -e VEEAM_USERNAME=u -e VEEAM_PASSWORD=p \
    veeam_licenses_exporter:gr-hc-test && \
  sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' veeam_gr_hc_test
```

  **Must print `healthy`.** On an amd64 host substitute `amd64` for `arm64`
  throughout; a mismatch produces `exec format error`, not a probe failure.

- [x] **Step 8: Tear down and clean up.**

```bash
docker rm -f veeam_gr_hc_test && docker rmi veeam_licenses_exporter:gr-hc-test && rm -rf /tmp/veeam_grctx
```

- [x] **Step 9: Commit.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && git add Dockerfile Dockerfile.goreleaser && git commit -m "feat(docker)!: Alpine release image at uid 10001, HEALTHCHECK on /livez

BREAKING CHANGE: the published image moves from gcr.io/distroless/static:nonroot
(uid 65532) to alpine:latest running as the named user 'licenses' at uid 10001,
matching the local Dockerfile and the rest of the exporter family. Anyone pinning
the container uid — a securityContext runAsUser, a volume's file ownership — must
update it. Both Dockerfiles gain a HEALTHCHECK against /livez on 9107.

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 16: Compose healthchecks

**Files:**
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/docker-compose.yml`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/docker-compose.ghcr.yml`

**Interfaces:**
- Consumes: `/livez` on 9107; the Alpine image from Task 15.
- Produces: a `healthcheck:` on the exporter service in both files, with values
  identical to the Dockerfile's.

- [x] **Step 1: Add the healthcheck to `docker-compose.yml`.** In the
  `veeam_licenses_exporter` service, replace:

```yaml
      - VEEAM_PASSWORD=${VEEAM_PASSWORD:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - VEEAM_PASSWORD=${VEEAM_PASSWORD:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9107/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 2: Add the same healthcheck to `docker-compose.ghcr.yml`.** In the
  `veeam_licenses_exporter` service, replace:

```yaml
      - VEEAM_PASSWORD=${VEEAM_PASSWORD:-}
    restart: unless-stopped

  prometheus:
```

  with:

```yaml
      - VEEAM_PASSWORD=${VEEAM_PASSWORD:-}
    healthcheck:
      # 127.0.0.1, never localhost: busybox wget resolves localhost via ::1 first
      # and the exporter binds IPv4 only. Values must match the Dockerfile's
      # HEALTHCHECK exactly — timeout is 5s in both places. Requires the first
      # release carrying the Alpine image (ADR-0002) or later: every published
      # image before it is distroless and carries no wget.
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://127.0.0.1:9107/livez"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    restart: unless-stopped

  prometheus:
```

- [x] **Step 3: Validate both files.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  docker compose -f docker-compose.yml config -q && \
  docker compose -f docker-compose.ghcr.yml config -q && echo "both valid"
```

- [x] **Step 4: Confirm the 5s timeout appears in every one of the four places.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  grep -n "timeout=5s\|timeout: 5s" Dockerfile Dockerfile.goreleaser docker-compose.yml docker-compose.ghcr.yml
```

  Expect exactly four hits, one per file.

- [x] **Step 5: Bring the local stack up and assert the exporter is `healthy`.**
  Not the ghcr stack — see Global Constraint 9.

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  docker compose up -d veeam_licenses_exporter && sleep 45 && \
  docker inspect --format='{{.State.Health.Status}}' veeam_licenses_exporter
```

  **Must print `healthy`.**

- [x] **Step 6: Tear down.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && docker compose down
```

- [x] **Step 7: Commit.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && git add docker-compose.yml docker-compose.ghcr.yml && git commit -m "feat(compose): healthcheck the exporter against /livez on 9107

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 17: ADR, index row, nav entry, CHANGELOG, docs sweep

**Files:**
- Create: `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md`
  *(confirm `0002` is next in Step 1)*
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/index.md`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/mkdocs.yml`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/CHANGELOG.md`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/docs/deployment/docker.md`
- Modify: `/Users/fjacquet/Projects/veeam_licenses_exporter/README.md`

**Interfaces:**
- Consumes: Tasks 14–16.
- Produces: a numbered ADR, an index row, an `mkdocs.yml` nav entry, CHANGELOG
  entries under the **existing** `## [Unreleased]` heading (line 7), and docs that
  no longer claim 503-on-startup or a distroless image.

- [x] **Step 1: Confirm the next ADR number. Never assume.**

```bash
ls /Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/
```

  Expect `0001-consume-core-resty-em.md` and `index.md`, making **`0002`** next.
  If the highest is different, use *that* number + 1 throughout.

- [x] **Step 2: Create the ADR** at
  `docs/adr/0002-livez-readyz-probes-and-alpine-release-image.md`:

```markdown
# 2. `/livez` + `/readyz` probes and an Alpine release image

Date: 2026-08-01

## Status
Accepted

## Context
Two family-wide standards completed on 2026-08-01 — the always-200 `/livez` +
`/readyz` probe pattern, and the Alpine container-image standard — both skipped
this repo, because the `exporter-standards` skill's family table listed eight
repos and this one was not among them.

Concretely, two defects followed. First, `/health` (served by
`licenses-exporter-core`) returned **503 `starting`** until the first collection
cycle completed. Against a slow or unreachable Enterprise Manager that window is
long enough for a Kubernetes `livenessProbe` to restart a perfectly healthy
process, and long enough for a Docker `HEALTHCHECK` to report the container
unhealthy throughout start-up. Second, the published image was
`gcr.io/distroless/static:nonroot`, which has no shell and no `wget` — so the
image *could not carry a `HEALTHCHECK` at all*, while the local `./Dockerfile`
was already Alpine at uid 10001. The two build paths disagreed about the runtime.

All three licenses exporters share their HTTP wiring through
`licenses-exporter-core`: it owns the only `http.ServeMux` and the only `/health`
handler, and no consumer registers a route. So the probe half is one library
change plus three dependency bumps.

## Decision
Consume `licenses-exporter-core` **v1.1.0**, which serves `/livez` and `/readyz`
from a handler that reads no state and makes `/health` answer 200
unconditionally, keeping `starting`/`ok` as body content.

Convert `Dockerfile.goreleaser` from `gcr.io/distroless/static:nonroot` to
`alpine:latest`, running as the named user `licenses` at uid **10001** — matching
the local `./Dockerfile` and the rest of the exporter family. Add a `HEALTHCHECK`
against `http://127.0.0.1:9107/livez` to both Dockerfiles and a matching
`healthcheck:` to both compose files.

`127.0.0.1` and never `localhost`: busybox `wget` resolves `localhost` via `::1`
first and this exporter binds IPv4 only, so a `localhost` check fails at runtime
while passing both `hadolint` and `docker compose config`.

Probes never point at `/metrics`: rendering the full exposition per probe tick is
needless load and can block behind a slow collection cycle.

## Consequences
- **Breaking:** the published container's uid moves `65532` → `10001`. Anyone
  pinning it — a `securityContext.runAsUser`, ownership on a mounted secret or
  log volume — must update. This repo ships no Helm chart, so there is no chart
  default to change.
- The release image gains a shell and busybox. That is a deliberate trade: a
  working `HEALTHCHECK` and one base image across fifteen repos, bought with a
  larger attack surface than distroless.
- `alpine:latest` is unpinned, family-wide. It is the one build input whose
  contents can change between two builds of the same commit. Uniformity was
  chosen over reproducibility; revisiting it is a fifteen-repo decision, not a
  per-repo one.
- Anything asserting a 503 from `/health` — an alert rule, a smoke test, a
  blackbox-exporter check — must move to reading the body, or to `/readyz`.
- Engine behaviour arrives by a core version bump, not a local edit (ADR-0001).
```

- [x] **Step 3: Add the index row.** In `docs/adr/index.md`, after the `0001` row
  (line 10), insert:

```markdown
| [0002](0002-livez-readyz-probes-and-alpine-release-image.md) | `/livez` + `/readyz` probes and an Alpine release image at uid 10001 | accepted |
```

  and update the closing sentence — replace:

```markdown
To add a decision, copy [`0001`](0001-consume-core-resty-em.md)'s structure to
the next number and link it here.
```

  with:

```markdown
To add a decision, copy [`0002`](0002-livez-readyz-probes-and-alpine-release-image.md)'s
structure to the next number and link it here.
```

- [x] **Step 4: Add the mkdocs nav entry.** In `mkdocs.yml`, after
  `- 0001 Consume core, resty EM client: adr/0001-consume-core-resty-em.md`
  (line 48), insert:

```yaml
      - 0002 /livez + /readyz probes, Alpine release image: adr/0002-livez-readyz-probes-and-alpine-release-image.md
```

- [x] **Step 5: Fill in the existing `## [Unreleased]` CHANGELOG section.** This
  file already has `## [Unreleased]` on line 7, immediately followed by
  `## [0.1.2] - 2026-07-10` on line 9. Insert this content between them:

```markdown
### Breaking
- The published container image runs as uid **10001** (named user `licenses`),
  not `65532`. `Dockerfile.goreleaser` moves from `gcr.io/distroless/static:nonroot`
  to `alpine:latest`, matching the local `./Dockerfile` and the rest of the
  exporter family. Anyone pinning the container uid — a `securityContext.runAsUser`,
  ownership on a mounted secret or log volume — must update it. See ADR-0002.

### Added
- **`/livez` and `/readyz`**, both always 200 and reading no state, via
  `licenses-exporter-core` v1.1.0. Point Kubernetes probes and container
  healthchecks at these, never at `/metrics`.
- **`HEALTHCHECK`** against `http://127.0.0.1:9107/livez` in both `Dockerfile` and
  `Dockerfile.goreleaser`, and a matching `healthcheck:` in `docker-compose.yml`
  and `docker-compose.ghcr.yml`. The ghcr healthcheck needs an image from this
  release or later — earlier published images are distroless and carry no `wget`.

### Changed
- **`/health` now always returns 200**, with `starting`/`ok` as the body rather
  than the status code. It previously returned 503 until the first collection
  cycle completed, which restarted healthy pods under a `livenessProbe` and
  reported containers unhealthy for the whole start-up window. Anything asserting
  a 503 from `/health` must be updated.
```

- [x] **Step 6: Update `docs/deployment/docker.md`.** Replace lines 30–32:

```markdown
`/metrics` and `/health` are both served on `9107`; `/health` returns HTTP 200 with
`starting` until the first collection cycle completes for every enabled Enterprise Manager,
then `ok`.
```

  with:

```markdown
Four routes are served on `9107`:

| Path | Status | Body |
|---|---|---|
| `/metrics` | 200 | Prometheus exposition |
| `/health` | always 200 | `starting` until the first collection cycle completes for every enabled Enterprise Manager, then `ok` |
| `/livez` | always 200 | empty |
| `/readyz` | always 200 | empty |

Point Kubernetes probes and container healthchecks at `/livez` and `/readyz` —
never at `/metrics`, which renders the whole exposition per probe tick and can
block behind a slow collection cycle. Both Dockerfiles ship a `HEALTHCHECK`
against `http://127.0.0.1:9107/livez` (`127.0.0.1`, not `localhost`: busybox
`wget` tries `::1` first and the exporter binds IPv4 only), and both compose
files carry the matching `healthcheck:`.
```

  Then check the file's opening paragraph with
  `sed -n '1,12p' docs/deployment/docker.md`: if it describes only the local
  `Dockerfile`, extend it to cover both images, as:

```markdown
Both images are non-root Alpine builds running as the unprivileged `licenses`
user (uid `10001`), listening on `9107` and reading `config.yaml` from
`/etc/veeam_licenses_exporter/config.yaml`. `Dockerfile` is the multi-stage build
from source; `Dockerfile.goreleaser` is the release image published to GHCR,
which copies the prebuilt binary. Published images before the ADR-0002 release
were `gcr.io/distroless/static:nonroot` at uid `65532`.
```

- [x] **Step 7: Update `README.md`.** Replace line 55:

```markdown
# metrics: http://localhost:9107/metrics   health: http://localhost:9107/health
```

  with:

```markdown
# metrics: http://localhost:9107/metrics   health: http://localhost:9107/health
# probes:  http://localhost:9107/livez     http://localhost:9107/readyz  (always 200)
```

- [x] **Step 8: Sweep for falsified claims.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  grep -rn "distroless\|65532\|nonroot" \
    --include="*.md" --include="*.yml" --include="*.yaml" --include="Dockerfile*" --include="*.go" . \
  | grep -v "^./site/"
```

  The only acceptable remaining hits are inside `docs/adr/`, `CHANGELOG.md`, and
  the `docs/deployment/docker.md` sentence added in Step 6 — all of which describe
  the *past* state explicitly. Any other hit is a stale user-facing claim: fix it.

- [x] **Step 9: Build the docs site strictly.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict
```

  Must exit 0.

- [x] **Step 10: Run the gate one more time.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && make ci
```

- [x] **Step 11: Commit.**

```bash
cd /Users/fjacquet/Projects/veeam_licenses_exporter && \
  git add docs/adr CHANGELOG.md mkdocs.yml docs/deployment/docker.md README.md && \
  git commit -m "docs: ADR-0002 for the probes + Alpine release image, changelog, doc sweep

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Self-Review

Run every item before declaring the plan complete. Each is a command with an
expected output — evidence, not assertion.

- [x] **Core serves all four routes and never 503s.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && \
  grep -n "livez\|readyz\|staticOKHandler" server.go && \
  grep -n "StatusServiceUnavailable" health.go server.go
```

  Expect the first grep to hit; the second must print **nothing**.

- [x] **Core's gate is green and the tag is published.**

```bash
cd /Users/fjacquet/Projects/licenses-exporter-core && make ci && git ls-remote --tags origin | grep v1.1.0
```

- [x] **All three consumers pin core v1.1.0.**

```bash
for r in m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  printf '%s: ' "$r"; grep "licenses-exporter-core" /Users/fjacquet/Projects/$r/go.mod
done
```

  All three must show `v1.1.0`. Zero must show `v1.0.1`.

- [x] **No `localhost` anywhere in a healthcheck.**

```bash
for r in m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  grep -n "localhost" /Users/fjacquet/Projects/$r/Dockerfile \
    /Users/fjacquet/Projects/$r/Dockerfile.goreleaser \
    /Users/fjacquet/Projects/$r/docker-compose.yml \
    /Users/fjacquet/Projects/$r/docker-compose.ghcr.yml
done
```

  Must print **nothing**. (`README.md` may legitimately still say `localhost` in
  a browser URL — that is not a healthcheck.)

- [x] **The 5s timeout is in all twelve places (4 files × 3 repos).**

```bash
for r in m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  printf '%s: ' "$r"
  grep -c "timeout=5s\|timeout: 5s" /Users/fjacquet/Projects/$r/Dockerfile \
    /Users/fjacquet/Projects/$r/Dockerfile.goreleaser \
    /Users/fjacquet/Projects/$r/docker-compose.yml \
    /Users/fjacquet/Projects/$r/docker-compose.ghcr.yml | tr '\n' ' '; echo
done
```

  Every file must report `1`. Any `0` or `2` is a defect. Cross-check that no
  `timeout: 10s` survives:
  `grep -rn "timeout: 10s\|timeout=10s" /Users/fjacquet/Projects/{m365,vmware,veeam}_licenses_exporter/docker-compose*.yml /Users/fjacquet/Projects/{m365,vmware,veeam}_licenses_exporter/Dockerfile*`
  must print nothing.

- [x] **Each repo's `HEALTHCHECK` targets its own port.**

```bash
grep -h "127.0.0.1" /Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/m365_licenses_exporter/docker-compose*.yml | grep -c 9105
grep -h "127.0.0.1" /Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/vmware_licenses_exporter/docker-compose*.yml | grep -c 9106
grep -h "127.0.0.1" /Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/veeam_licenses_exporter/docker-compose*.yml | grep -c 9107
```

  Each must print `4`. A cross-wired port (m365 checking 9106) is the single most
  likely copy-paste defect in this plan.

- [x] **No `distroless`, `65532` or `nonroot` survives outside ADRs, CHANGELOGs and
  the explicit "before this release" doc sentences.**

```bash
for r in m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  echo "=== $r ==="
  grep -rn "distroless\|65532\|nonroot" \
    --include="*.md" --include="*.yml" --include="*.yaml" --include="Dockerfile*" --include="*.go" \
    /Users/fjacquet/Projects/$r | grep -v "/site/" | grep -v "/docs/adr/" | grep -v "CHANGELOG.md"
done
```

  The only permitted hits are the `docs/deployment/docker.md` sentences that
  explicitly describe the pre-release state. Anything else is a stale claim.

- [x] **No inline suppressions were added.**

```bash
grep -rn "hadolint ignore\|nosemgrep\|//nolint" \
  /Users/fjacquet/Projects/licenses-exporter-core \
  /Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile* 2>/dev/null | grep -v "/site/"
```

  Must print nothing new.

- [x] **No literal placeholder text was committed.**

```bash
grep -rn "ADR-000N\|<port>\|TBD" \
  /Users/fjacquet/Projects/m365_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/vmware_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/veeam_licenses_exporter/Dockerfile* \
  /Users/fjacquet/Projects/m365_licenses_exporter/docs/adr \
  /Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr \
  /Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr
```

  Must print nothing.

- [x] **Every new ADR has both an index row and a nav entry.**

```bash
grep -c "0011-livez-readyz" /Users/fjacquet/Projects/m365_licenses_exporter/docs/adr/index.md \
  /Users/fjacquet/Projects/m365_licenses_exporter/mkdocs.yml
grep -c "0002-livez-readyz" /Users/fjacquet/Projects/vmware_licenses_exporter/docs/adr/index.md \
  /Users/fjacquet/Projects/vmware_licenses_exporter/mkdocs.yml
grep -c "0002-livez-readyz" /Users/fjacquet/Projects/veeam_licenses_exporter/docs/adr/index.md \
  /Users/fjacquet/Projects/veeam_licenses_exporter/mkdocs.yml
```

  Every count must be `≥1`. (Adjust the numbers if Step 1 of Tasks 9/13/17 found
  different next-numbers.)

- [x] **Every CHANGELOG has an `## [Unreleased]` section carrying this work.**

```bash
for r in m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  printf '%s: ' "$r"
  grep -c "^## \[Unreleased\]" /Users/fjacquet/Projects/$r/CHANGELOG.md
done
grep -c "^## \[Unreleased\]" /Users/fjacquet/Projects/licenses-exporter-core/CHANGELOG.md
```

  All four must print `1`. m365's was **created** by this work; core's was too.

- [x] **All four repos have clean trees and green gates.**

```bash
for r in licenses-exporter-core m365_licenses_exporter vmware_licenses_exporter veeam_licenses_exporter; do
  echo "=== $r ==="
  git -C /Users/fjacquet/Projects/$r status --porcelain
  (cd /Users/fjacquet/Projects/$r && make ci >/dev/null 2>&1 && echo "ci OK" || echo "ci FAILED")
done
```

  Every `status --porcelain` must be empty (ignoring untracked `site/` and
  `coverage.out` if they are gitignored) and every gate must print `ci OK`.

- [ ] **Runtime health was actually asserted, not just read.** *(Left unticked by
  the 2026-08-01 paperwork pass: this session found no direct evidence — no
  running or dangling `hc-test`/`gr-hc-test` images for m365/vmware/veeam — that
  the six `docker inspect` checks were run. Their absence is also consistent
  with the plan's own teardown steps having succeeded, so this is genuinely
  unconfirmed either way, not disproved.)* Confirm you ran the
  `docker inspect --format='{{.State.Health.Status}}'` check and saw `healthy` for
  **six** images: the local and the goreleaser image of each of the three repos
  (Tasks 7/11/15 Steps 4 and 7). If you skipped any because "the Dockerfile looks
  right", go back and run it — that is the exact shortcut that shipped the
  `localhost`/`::1` bug.

- [ ] **Out-of-scope work was not started.** *(Left unticked by the 2026-08-01
  paperwork pass: `kemp_exporter`, `nsr_exporter`, `pve_exporter`, and
  `idrac_exporter` all show a commit dated 2026-08-01 today — e.g. `nsr_exporter`
  `b8d995d docs(plan): tick completed Task 11 and Self-Review checkboxes`,
  `pve_exporter e91ca28 docs(changelog): record the startup-ordering and
  server.uri fixes`. These read as a different, concurrent family-wide effort
  (health-handler/startup-ordering work per sibling sessions, per the
  "siblings are being modified concurrently" constraint on this session) rather
  than this plan's Phase 1–4 scope, but this session has no way to confirm that
  boundary from the commit messages alone — re-verify against the actual spec
  before re-ticking.)* Plans B–E of the spec
  (`kemp_exporter` port move, `nsr_exporter`, `pve_exporter`, `idrac_exporter`)
  and the `exporter-standards` skill correction are **not** part of this plan.
  Confirm no commits landed in those repos:

```bash
for r in kemp_exporter nsr_exporter pve_exporter idrac_exporter; do
  printf '%s: ' "$r"; git -C /Users/fjacquet/Projects/$r log --oneline -1 2>/dev/null || echo "(absent)"
done
```

  None should show a commit dated today from this effort.
