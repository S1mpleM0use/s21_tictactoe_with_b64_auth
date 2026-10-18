package storage

import (
	"context"
	domainModel "tictactoe/internal/domain/model"

	"github.com/google/uuid"
)

type GameRepository interface {
	SaveGame(ctx context.Context, game domainModel.Game) error
	GetGameByID(ctx context.Context, id uuid.UUID) (domainModel.Game, error)
	GetGamesByState(ctx context.Context, state string) ([]domainModel.Game, error)
	GetCurrentGame(ctx context.Context, userID uuid.UUID) (domainModel.Game, error)
}
