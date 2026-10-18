package app

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	domainModel "tictactoe/internal/domain/model"
	"tictactoe/internal/infrastructure/datasource/user"

	"github.com/google/uuid"
)

type mockUserRepo struct {
	usersByID    map[uuid.UUID]domainModel.User
	usersByLogin map[string]domainModel.User
	saveErr      error
	getErr       error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		usersByID:    make(map[uuid.UUID]domainModel.User),
		usersByLogin: make(map[string]domainModel.User),
	}
}

func (m *mockUserRepo) SaveUser(_ context.Context, u domainModel.User) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if _, exists := m.usersByLogin[u.Login]; exists {
		return user.ErrLoginAlreadyTaken
	}
	m.usersByID[u.ID] = u
	m.usersByLogin[u.Login] = u
	return nil
}

func (m *mockUserRepo) GetUserByLogin(_ context.Context, login string) (domainModel.User, error) {
	if m.getErr != nil {
		return domainModel.User{}, m.getErr
	}
	u, ok := m.usersByLogin[login]
	if !ok {
		return domainModel.User{}, user.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetUserByID(_ context.Context, id uuid.UUID) (domainModel.User, error) {
	if m.getErr != nil {
		return domainModel.User{}, m.getErr
	}
	u, ok := m.usersByID[id]
	if !ok {
		return domainModel.User{}, user.ErrUserNotFound
	}
	return u, nil
}

func basicHeader(login, password string) string {
	raw := login + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

func TestAuthAndUserService_RegisterErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	userSvc := NewUserService(repo)
	authSvc := NewAuthService(userSvc)

	tests := []struct {
		name    string
		req     domainModel.SignUpRequest
		wantErr error
	}{
		{
			name:    "empty login",
			req:     domainModel.SignUpRequest{Login: "   ", Password: "password123"},
			wantErr: ErrLoginRequired,
		},
		{
			name:    "login too long (>20 runes)",
			req:     domainModel.SignUpRequest{Login: strings.Repeat("a", 21), Password: "password123"},
			wantErr: ErrLoginTooLong,
		},
		{
			name:    "password too short (<4 chars)",
			req:     domainModel.SignUpRequest{Login: "nikolai", Password: "123"},
			wantErr: ErrPasswordTooShort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := authSvc.Register(ctx, tt.req)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}

	t.Run("successful registration and duplicate login check", func(t *testing.T) {
		req := domainModel.SignUpRequest{Login: "player1", Password: "secret"}
		created, err := authSvc.Register(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if created.ID == uuid.Nil || created.Login != "player1" {
			t.Fatalf("unexpected created user: %+v", created)
		}

		// Повторная регистрация с тем же логином
		_, err = authSvc.Register(ctx, req)
		if !errors.Is(err, ErrLoginAlreadyTaken) {
			t.Fatalf("expected ErrLoginAlreadyTaken, got %v", err)
		}
	})

	t.Run("race condition duplicate on SaveUser", func(t *testing.T) {
		raceRepo := newMockUserRepo()
		raceRepo.saveErr = user.ErrLoginAlreadyTaken
		svc := NewAuthService(NewUserService(raceRepo))

		_, err := svc.Register(ctx, domainModel.SignUpRequest{Login: "race_user", Password: "secret"})
		if !errors.Is(err, ErrLoginAlreadyTaken) {
			t.Fatalf("expected ErrLoginAlreadyTaken on SaveUser conflict, got %v", err)
		}
	})
}

func TestAuthService_AuthenticateErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	userSvc := NewUserService(repo)
	authSvc := NewAuthService(userSvc)

	registered, err := authSvc.Register(ctx, domainModel.SignUpRequest{
		Login:    "valid_user",
		Password: "correct_password",
	})
	if err != nil {
		t.Fatalf("setup register failed: %v", err)
	}

	tests := []struct {
		name    string
		header  string
		wantErr error
	}{
		{
			name:    "missing header",
			header:  "",
			wantErr: ErrAuthNotBasic,
		},
		{
			name:    "wrong scheme (Bearer instead of Basic)",
			header:  "Bearer some-token",
			wantErr: ErrAuthNotBasic,
		},
		{
			name:    "invalid base64 payload",
			header:  "Basic !!!not_base64!!!",
			wantErr: ErrAuthNotBase64,
		},
		{
			name:    "missing colon in decoded credentials",
			header:  "Basic " + base64.StdEncoding.EncodeToString([]byte("userwithoutcolon")),
			wantErr: ErrAuthFormat,
		},
		{
			name:    "empty login before colon",
			header:  basicHeader("", "password"),
			wantErr: ErrAuthFormat,
		},
		{
			name:    "empty password after colon",
			header:  basicHeader("valid_user", ""),
			wantErr: ErrAuthFormat,
		},
		{
			name:    "non-existent user",
			header:  basicHeader("ghost_user", "correct_password"),
			wantErr: ErrInvalidCredentials,
		},
		{
			name:    "wrong password",
			header:  basicHeader("valid_user", "wrong_password"),
			wantErr: ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := authSvc.Authenticate(ctx, tt.header)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}

	t.Run("successful authentication returns user UUID", func(t *testing.T) {
		id, err := authSvc.Authenticate(ctx, basicHeader("valid_user", "correct_password"))
		if err != nil {
			t.Fatalf("unexpected auth error: %v", err)
		}
		if id != registered.ID {
			t.Fatalf("expected user ID %s, got %s", registered.ID, id)
		}
	})
}

func TestUserService_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := newMockUserRepo()
	userSvc := NewUserService(repo)

	created, err := userSvc.Create(ctx, domainModel.SignUpRequest{
		Login:    "lookup_user",
		Password: "password123",
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	t.Run("existing user", func(t *testing.T) {
		found, err := userSvc.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.ID != created.ID || found.Login != "lookup_user" {
			t.Fatalf("unexpected user: %+v", found)
		}
	})

	t.Run("non-existent user returns ErrUserNotFound", func(t *testing.T) {
		_, err := userSvc.GetByID(ctx, uuid.New())
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})
}
