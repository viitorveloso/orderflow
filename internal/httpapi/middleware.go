package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yourusername/orderflow/internal/domain"
)

// contextKey is an unexported type for context keys so they cannot collide with
// keys set by other packages.
type contextKey string

const (
	ctxUserID contextKey = "user_id"
	ctxRole   contextKey = "role"
)

// requireAuth rejects requests without a valid bearer token. On success it puts
// the authenticated user's ID and role into the request context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing or malformed Authorization header"})
			return
		}
		claims, err := s.tokens.Parse(token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid or expired token"})
			return
		}
		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token subject"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxRole, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin enforces authentication and then an admin role. It composes
// requireAuth so the context is populated before the role check runs.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if roleFromContext(r.Context()) != domain.RoleAdmin {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// logging records one structured line per request with status and latency.
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		s.logger.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// recoverer converts a panic in any handler into a 500 instead of crashing the
// server, logging the cause.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered", "err", rec, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the response status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.status = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func userIDFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxUserID).(int64)
	return id
}

func roleFromContext(ctx context.Context) domain.Role {
	role, _ := ctx.Value(ctxRole).(domain.Role)
	return role
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	return token, token != ""
}
