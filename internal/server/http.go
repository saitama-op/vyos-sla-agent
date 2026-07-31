package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/saitama-op/vyos-sla-agent/internal/decision"
)

// APIServer serves HTTP endpoints for health checks and status queries
type APIServer struct {
	addr    string
	engines map[string]*decision.Engine
}

// NewAPIServer creates a new HTTP API server
func NewAPIServer(addr string, engines map[string]*decision.Engine) *APIServer {
	return &APIServer{
		addr:    addr,
		engines: engines,
	}
}

// Start registers the routes and begins listening on the configured address
func (s *APIServer) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)

	// Dynamically register an exact endpoint for every WAN loaded from the YAML config.
	// If a user queries /wanstatus/INVALID, Go's ServeMux automatically returns a 404.
	for name, engine := range s.engines {
		// Capture loop variables for the closure
		ispName := name
		ispEngine := engine

		route := "/wanstatus/" + ispName
		slog.Info("Registering WAN status endpoint", "route", route)

		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// Engine is safely bound via closure; no need to look it up or parse URLs
			state := ispEngine.CurrentState

			// Return 200 for UP, 503 for DEGRADED/DOWN
			if state == decision.StateUp {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("UP\n"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(string(state) + "\n"))
			}
		})
	}

	slog.Info("Starting HTTP API server", "address", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// handleHealth provides a simple liveliness probe for external monitors
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleStatus returns the current SD-WAN state of all tracked interfaces
func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build a map of WAN names to their current state (UP, DEGRADED, DOWN)
	statusMap := make(map[string]string)
	for name, engine := range s.engines {
		statusMap[name] = string(engine.CurrentState)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(statusMap); err != nil {
		slog.Error("Failed to encode status JSON", "error", err)
	}
}
