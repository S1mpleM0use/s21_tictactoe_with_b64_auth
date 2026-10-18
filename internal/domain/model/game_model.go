package model

import "github.com/google/uuid"

type Game struct {
	ID           uuid.UUID
	Board        Board
	GameState    string
	Mode         string
	HostID       uuid.UUID
	OpponentID   *uuid.UUID
	NextPlayerID *uuid.UUID
	Symbols      map[string]string
	WinnerID     *uuid.UUID
}
