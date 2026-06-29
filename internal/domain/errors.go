package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Sentinel errors returned by the service and repository layers. The HTTP layer
// maps each of these to an appropriate status code, keeping transport concerns
// out of the business logic.
var (
	ErrNotFound            = errors.New("resource not found")
	ErrConflict            = errors.New("resource already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrForbidden           = errors.New("operation not permitted")
	ErrInvalidStatusChange = errors.New("invalid status transition")
)

// FieldErrors accumulates per-field validation messages. It is returned as a
// single error so callers can branch on it with errors.As and render a 422
// response that tells the client exactly which fields are wrong.
type FieldErrors map[string]string

// Error implements the error interface with a deterministic, readable message.
func (fe FieldErrors) Error() string {
	keys := make([]string, 0, len(fe))
	for k := range fe {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(fe))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, fe[k]))
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Has reports whether any field errors have been recorded.
func (fe FieldErrors) Has() bool { return len(fe) > 0 }

// Add records a message for a field if one is not already present.
func (fe FieldErrors) Add(field, msg string) {
	if _, ok := fe[field]; !ok {
		fe[field] = msg
	}
}
