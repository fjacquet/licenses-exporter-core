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
