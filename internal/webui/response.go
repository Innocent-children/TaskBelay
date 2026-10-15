package webui

import (
	"encoding/json"
	"net/http"

	"github.com/Innocent-children/taskbelay/internal/protocol"
)

/**
 * HTTP failures use the same error contract as Core tools, including whether a
 * semantic action may already have happened. The status code is transport data.
 */
func WriteFailure(w http.ResponseWriter, status int, name string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(protocol.ErrorEnvelope(name, err))
}

func errorStatus(e protocol.Envelope) int {
	if e.Error == nil {
		return http.StatusInternalServerError
	}
	switch e.Error.Code {
	case "TASK_NOT_FOUND", "RUN_NOT_FOUND":
		return http.StatusNotFound
	case "INVALID_ARGUMENT":
		return http.StatusBadRequest
	case "LOCAL_SESSION_REQUIRED", "HOST_REJECTED":
		return http.StatusForbidden
	case "STORAGE_UNAVAILABLE", "CONTROL_INITIALIZATION_FAILED":
		return http.StatusServiceUnavailable
	case "INTERNAL_ERROR":
		return http.StatusInternalServerError
	default:
		return http.StatusConflict
	}
}
