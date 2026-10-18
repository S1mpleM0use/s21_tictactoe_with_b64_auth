package model

import (
	"github.com/google/uuid"
)

type GameResponse struct {
	ID           uuid.UUID         `json:"id"`
	Board        [][]int           `json:"board"`
	GameState    string            `json:"game_state"` // "waiting" | "playing" | "finished" | "draw"
	Mode         string            `json:"mode"`       // "VSplayer" | "VScomputer"
	HostID       uuid.UUID         `json:"host_id"`
	OpponentID   *uuid.UUID        `json:"opponent_id,omitempty"`
	NextPlayerID *uuid.UUID        `json:"next_player_id,omitempty"`
	WinnerID     *uuid.UUID        `json:"winner_id,omitempty"`
	Symbols      map[string]string `json:"symbols,omitempty"`
}

type MoveRequest struct {
	Row    int `json:"row"`
	Column int `json:"column"`
}

type CreateGameRequest struct {
	Mode string `json:"mode"` // "VSplayer" или "VScomputer"
}

type SignUpRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type User struct {
	ID    uuid.UUID `json:"id"`
	Login string    `json:"login"`
}
