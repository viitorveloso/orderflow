package domain

import "time"

// Role enumerates the access levels a user can hold. Authorization decisions in
// the service and HTTP layers branch on this value.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Valid reports whether r is a recognized role.
func (r Role) Valid() bool {
	return r == RoleUser || r == RoleAdmin
}

// User is an account that can authenticate and place orders. PasswordHash holds
// an encoded PBKDF2 hash (see internal/auth) and is never serialized to clients.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}
