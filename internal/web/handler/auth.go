package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"tictactoe/internal/app"

	"github.com/google/uuid"
)

type contextKey string

const userUUIDKey contextKey = "userUUID"

func userUUID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(userUUIDKey).(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, errors.New("no authenticated user in context")
	}
	return id, nil
}

type UserAuthenticator struct {
	authSvc app.AuthService
}

func NewUserAuthenticator(authSvc app.AuthService) *UserAuthenticator {
	return &UserAuthenticator{authSvc: authSvc}
}

func (a *UserAuthenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.authSvc.Authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			if isAuthenticationError(err) {
				w.Header().Set("WWW-Authenticate", `Basic realm="tic-tac-toe"`)
				writeJSONError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			log.Printf("authentication service error: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		ctx := context.WithValue(r.Context(), userUUIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *UserAuthenticator) authenticate(next http.Handler) http.Handler {
	return a.Authenticate(next)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("encode error response: %v", err)
	}
}
