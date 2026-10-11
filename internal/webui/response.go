package webui

import (
	"encoding/json"
	"net/http"
)

func WriteFailure(w http.ResponseWriter, status int, id, state string, f ErrorResponse, r RecoveryAdvice) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(map[string]any{"ok": false, "request_id": id, "error": f, "recovery": r, "write_state": state})
}
