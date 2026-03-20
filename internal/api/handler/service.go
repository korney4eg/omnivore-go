package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// ServiceHandler handles service endpoints.
type ServiceHandler struct{}

// NewServiceHandler creates a new service handler.
func NewServiceHandler() *ServiceHandler {
	return &ServiceHandler{}
}

// VersionResponse represents version info.
type VersionResponse struct {
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	Service   string `json:"service"`
}

// Version handles GET /_ah/version
func (h *ServiceHandler) Version(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	respondJSON(w, VersionResponse{
		Version:   "1.0.0", // TODO: Get from build info
		GoVersion: runtime.Version(),
		Service:   "omnivore-api",
	})
}

// Warmup handles GET /_ah/warmup
// Used by Google App Engine to pre-warm instances
func (h *ServiceHandler) Warmup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Just return success - actual warmup happens at server startup
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"ready": true})
}
