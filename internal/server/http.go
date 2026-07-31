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
		// Note: To be perfectly thread-safe, ensure you add a GetState() 
		// method to your decision.Engine that wraps CurrentState in its RWMutex.
		statusMap[name] = string(engine.CurrentState)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(statusMap); err != nil {
		slog.Error("Failed to encode status JSON", "error", err)
	}
}
