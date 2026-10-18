package handler

import (
	"net/http"
)

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// public
	mux.HandleFunc("POST /user", h.register)
	mux.HandleFunc("POST /user/login", h.login)

	// auth only
	mux.Handle("POST /game", h.authenticated(h.CreateGame))
	mux.Handle("GET /game/list", h.authenticated(h.GetWaitingGames))
	mux.Handle("GET /game/current", h.authenticated(h.GetCurrentGame))
	mux.Handle("GET /user/{userID}", h.authenticated(h.GetUserInfoByID))
	mux.Handle("POST /game/{gameID}/join", h.authenticated(h.JoinGame))
	mux.Handle("POST /game/{gameID}/move", h.authenticated(h.MakeMove))

	return mux
}
