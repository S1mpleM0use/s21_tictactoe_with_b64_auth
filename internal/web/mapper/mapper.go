package mapper

import (
	domainModel "tictactoe/internal/domain/model"
	webModel "tictactoe/internal/web/model"
)

func ToGameResponse(g domainModel.Game) webModel.GameResponse {
	return webModel.GameResponse{
		ID:           g.ID,
		Board:        g.Board,
		GameState:    g.GameState,
		Mode:         g.Mode,
		HostID:       g.HostID,
		OpponentID:   g.OpponentID,
		NextPlayerID: g.NextPlayerID,
		WinnerID:     g.WinnerID,
		Symbols:      g.Symbols,
	}
}

func UserToWeb(u domainModel.User) webModel.User {
	return webModel.User{ID: u.ID, Login: u.Login}
}
