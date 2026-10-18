package model

import "github.com/google/uuid"

type GameDTO struct {
	ID           uuid.UUID         `db:"id"`
	Board        BoardDTO          `db:"board"`
	GameState    string            `db:"state"`
	Mode         string            `db:"mode"`
	HostID       uuid.UUID         `db:"host_id"`
	OpponentID   *uuid.UUID        `db:"opp_id"`
	NextPlayerID *uuid.UUID        `db:"next_id"`
	Symbols      map[string]string `db:"symbols"`
	WinnerID     *uuid.UUID        `db:"winner_id"`
}
