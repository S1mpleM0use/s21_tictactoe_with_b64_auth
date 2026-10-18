package app

import (
	"context"
	domainModel "tictactoe/internal/domain/model"

	"github.com/google/uuid"
)

type AuthService interface {
	Register(ctx context.Context, req domainModel.SignUpRequest) (domainModel.User, error)
	Authenticate(ctx context.Context, header string) (uuid.UUID, error)
}
