package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"tictactoe/internal/app"
	domainModel "tictactoe/internal/domain/model"
	webMapper "tictactoe/internal/web/mapper"
	webModel "tictactoe/internal/web/model"

	"github.com/google/uuid"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

type Handler struct {
	appService    app.AppService
	authService   app.AuthService
	userService   app.UserService
	authenticator *UserAuthenticator
}

func NewHandler(
	appService app.AppService,
	authService app.AuthService,
	userService app.UserService,
	authenticator *UserAuthenticator,
) *Handler {
	return &Handler{
		appService:    appService,
		authService:   authService,
		userService:   userService,
		authenticator: authenticator,
	}
}

// --- HTTP helpers ---

func (h *Handler) writeError(w http.ResponseWriter, status int, err error) {
	message := "unknown error"
	if err != nil {
		message = err.Error()
	}
	writeJSONError(w, status, message)
}

func (h *Handler) writeInternalError(w http.ResponseWriter, operation string, err error) {
	log.Printf("%s: %v", operation, err)
	writeJSONError(w, http.StatusInternalServerError, "internal server error")
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, err error, message string) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	writeJSONError(w, http.StatusBadRequest, message)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		h.writeInternalError(w, "encode JSON response", err)
		return
	}
	body = append(body, '\n')
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return errors.New("request body is empty")
	}
	if body[0] != '{' {
		return errors.New("expected a JSON object")
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON object")
		}
		return fmt.Errorf("unexpected trailing data: %w", err)
	}
	return nil
}

func parsePathUUID(r *http.Request, key string) (uuid.UUID, error) {
	value := r.PathValue(key)
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid %s: %w", key, err)
	}
	return id, nil
}

func (h *Handler) authenticated(next http.HandlerFunc) http.Handler {
	return h.authenticator.Authenticate(next)
}

// --- public endpoints ---

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req webModel.SignUpRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeDecodeError(w, err, "invalid request body: expected login and password")
		return
	}

	user, err := h.authService.Register(r.Context(), domainModel.SignUpRequest{
		Login:    req.Login,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, app.ErrLoginRequired),
			errors.Is(err, app.ErrPasswordTooShort),
			errors.Is(err, app.ErrLoginTooLong):
			h.writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, app.ErrLoginAlreadyTaken):
			h.writeError(w, http.StatusConflict, err)
		default:
			h.writeInternalError(w, "register user", err)
		}
		return
	}

	h.writeJSON(w, http.StatusCreated, webMapper.UserToWeb(user))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	userID, err := h.authService.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if isAuthenticationError(err) {
			w.Header().Set("WWW-Authenticate", `Basic realm="tic-tac-toe"`)
			h.writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		h.writeInternalError(w, "authenticate user", err)
		return
	}

	user, err := h.userService.GetByID(r.Context(), userID)
	if err != nil {
		h.writeInternalError(w, "get authenticated user", err)
		return
	}

	h.writeJSON(w, http.StatusOK, webMapper.UserToWeb(user))
}

// --- game endpoints ---

func (h *Handler) CreateGame(w http.ResponseWriter, r *http.Request) {
	userID, err := userUUID(r.Context())
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	var req webModel.CreateGameRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeDecodeError(w, err, "invalid request body: expected mode")
		return
	}

	game, err := h.appService.CreateGame(r.Context(), userID, req.Mode)
	if err != nil {
		if errors.Is(err, app.ErrInvalidMode) {
			h.writeError(w, http.StatusBadRequest, err)
		} else {
			h.writeInternalError(w, "create game", err)
		}
		return
	}

	h.writeJSON(w, http.StatusCreated, webMapper.ToGameResponse(game))
}

func (h *Handler) JoinGame(w http.ResponseWriter, r *http.Request) {
	userID, err := userUUID(r.Context())
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	gameID, err := parsePathUUID(r, "gameID")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, errors.New("invalid game id"))
		return
	}

	game, err := h.appService.JoinGame(r.Context(), gameID, userID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrGameNotFound):
			h.writeError(w, http.StatusNotFound, err)
		case errors.Is(err, app.ErrNotVSPlayer), errors.Is(err, app.ErrInvalidMode):
			h.writeError(w, http.StatusBadRequest, err)
		case errors.Is(err, app.ErrNotWaiting):
			h.writeError(w, http.StatusConflict, err)
		case errors.Is(err, app.ErrCreatorCannotJoin):
			h.writeError(w, http.StatusForbidden, err)
		default:
			h.writeInternalError(w, "join game", err)
		}
		return
	}

	h.writeJSON(w, http.StatusOK, webMapper.ToGameResponse(game))
}

func (h *Handler) MakeMove(w http.ResponseWriter, r *http.Request) {
	userID, err := userUUID(r.Context())
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	gameID, err := parsePathUUID(r, "gameID")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, errors.New("invalid game id"))
		return
	}

	var req webModel.MoveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.writeDecodeError(w, err, "invalid request body: expected row and column")
		return
	}

	game, err := h.appService.ApplyMove(r.Context(), gameID, userID, req.Row, req.Column)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrGameNotFound):
			h.writeError(w, http.StatusNotFound, err)
		case errors.Is(err, app.ErrNotYourGame):
			h.writeError(w, http.StatusForbidden, err)
		case errors.Is(err, app.ErrNotYourTurn), errors.Is(err, app.ErrGameNotPlaying), errors.Is(err, app.ErrCellOccupied):
			h.writeError(w, http.StatusConflict, err)
		case errors.Is(err, app.ErrCellOutOfBounds), errors.Is(err, app.ErrInvalidMove):
			h.writeError(w, http.StatusBadRequest, err)
		default:
			h.writeInternalError(w, "apply move", err)
		}
		return
	}

	h.writeJSON(w, http.StatusOK, webMapper.ToGameResponse(game))
}

func (h *Handler) GetWaitingGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.appService.GetWaitingGames(r.Context())
	if err != nil {
		h.writeInternalError(w, "get waiting games", err)
		return
	}

	responses := make([]webModel.GameResponse, 0, len(games))
	for _, game := range games {
		responses = append(responses, webMapper.ToGameResponse(game))
	}
	h.writeJSON(w, http.StatusOK, responses)
}

func (h *Handler) GetCurrentGame(w http.ResponseWriter, r *http.Request) {
	userID, err := userUUID(r.Context())
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	game, err := h.appService.GetCurrentGame(r.Context(), userID)
	if err != nil {
		if errors.Is(err, app.ErrGameNotFound) {
			h.writeError(w, http.StatusNotFound, err)
		} else {
			h.writeInternalError(w, "get current game", err)
		}
		return
	}

	h.writeJSON(w, http.StatusOK, webMapper.ToGameResponse(game))
}

func (h *Handler) GetUserInfoByID(w http.ResponseWriter, r *http.Request) {
	userID, err := parsePathUUID(r, "userID")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, errors.New("invalid user id"))
		return
	}

	user, err := h.userService.GetByID(r.Context(), userID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUserNotFound):
			h.writeError(w, http.StatusNotFound, err)
		default:
			h.writeInternalError(w, "get user by id", err)
		}
		return
	}

	h.writeJSON(w, http.StatusOK, webMapper.UserToWeb(user))
}
