package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/yourusername/orderflow/internal/domain"
)

// maxBodyBytes caps request bodies to protect against unbounded payloads.
const maxBodyBytes = 1 << 20 // 1 MiB

// errorResponse is the uniform error envelope returned to clients. Fields is
// populated only for validation (422) responses.
type errorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields,omitempty"`
}

// writeJSON encodes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// decodeJSON reads a JSON request body into dst, rejecting bodies that are too
// large or contain unknown fields. Unknown-field rejection catches client typos
// early instead of silently ignoring them.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// writeError translates an application error into the right HTTP status and a
// safe JSON body. Transport concerns live here so the service layer can speak
// purely in domain sentinel errors. Unexpected errors are logged with detail
// but reported generically to avoid leaking internals.
func writeError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var fields domain.FieldErrors

	switch {
	case errors.As(err, &fields):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error:  "validation failed",
			Fields: fields,
		})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "resource not found"})
	case errors.Is(err, domain.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "resource already exists"})
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
	case errors.Is(err, domain.ErrInsufficientStock):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "insufficient stock"})
	case errors.Is(err, domain.ErrInvalidStatusChange):
		writeJSON(w, http.StatusConflict, errorResponse{Error: "invalid status transition"})
	case errors.Is(err, domain.ErrForbidden):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "operation not permitted"})
	default:
		logger.Error("unhandled error", "err", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}
