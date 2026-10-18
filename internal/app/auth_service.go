package app

import (
	"context"
	"encoding/base64"
	"strings"

	domainModel "tictactoe/internal/domain/model"

	"github.com/google/uuid"
)

type authService struct {
	userService UserService
}

func NewAuthService(userService UserService) AuthService {
	return &authService{userService: userService}
}

func (s *authService) Register(ctx context.Context, req domainModel.SignUpRequest) (domainModel.User, error) {
	return s.userService.Create(ctx, req)
}

func (s *authService) Authenticate(ctx context.Context, header string) (uuid.UUID, error) {
	login, password, err := parseBasicHeader(header)
	if err != nil {
		return uuid.Nil, err
	}

	authenticatedUser, err := s.userService.VerifyCredentials(ctx, login, password)
	if err != nil {
		return uuid.Nil, err
	}

	return authenticatedUser.ID, nil
}

func base64Decode(value string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func parseBasicHeader(header string) (login, password string, err error) {
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return "", "", ErrAuthNotBasic
	}

	decoded, err := base64Decode(strings.TrimPrefix(header, prefix))
	if err != nil {
		return "", "", ErrAuthNotBase64
	}

	parts := strings.SplitN(decoded, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrAuthFormat
	}
	return parts[0], parts[1], nil
}
