package httpapi

import (
	"net/http"

	"github.com/yourusername/orderflow/internal/service"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	user, err := s.auth.Register(r.Context(), service.RegisterInput{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	token, user, err := s.auth.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		writeError(w, s.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}
