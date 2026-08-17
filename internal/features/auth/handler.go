package auth

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"automotive-workshop-api/internal/shared/httpx"
	"automotive-workshop-api/internal/shared/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

type meResponse struct {
	ID    string `json:"id"`
	Code  int64  `json:"code"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "email and password are required")
		return
	}
	result, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}
	if err != nil {
		// err carries no password/token (BR5) — safe to log.
		log.Printf("auth: login failed: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, loginResponse{
		AccessToken: result.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   result.ExpiresIn,
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserID(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing authentication")
		return
	}
	user, err := h.svc.UserByID(r.Context(), userID)
	if errors.Is(err, ErrUserNotFound) {
		httpx.Error(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}
	if err != nil {
		log.Printf("auth: me failed: %v", err)
		httpx.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, meResponse{ID: user.ID, Code: user.Code, Name: user.Name, Email: user.Email})
}
