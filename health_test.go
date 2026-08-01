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
