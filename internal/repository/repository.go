// Package repository contains the Postgres implementations of the repository
// interfaces declared in the service package. Each type wraps a *sql.DB and
// translates database errors into the domain sentinel errors (ErrNotFound,
// ErrConflict, ErrInsufficientStock) the rest of the app understands.
package repository

import (
	"errors"

	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE code Postgres returns for a unique
// constraint violation.
const pgUniqueViolation = "23505"

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation, so callers can map it to domain.ErrConflict.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == pgUniqueViolation
	}
	return false
}
