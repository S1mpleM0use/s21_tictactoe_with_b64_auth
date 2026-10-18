package app

import (
	"context"
	"errors"
	"testing"

	domainModel "tictactoe/internal/domain/model"
	"tictactoe/internal/domain/service"
	"tictactoe/internal/infrastructure/datasource/storage"

	"github.com/google/uuid"
)

type mockGameRepo struct {
	games   map[uuid.UUID]domainModel.Game
	saveErr error
	loadErr error
}

func newMockGameRepo() *mockGameRepo {
	return &mockGameRepo{
		games: make(map[uuid.UUID]domainModel.Game),
	}
}

func (m *mockGameRepo) SaveGame(_ context.Context, game domainModel.Game) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	// Копируем доску, чтобы мутации в тестах не меняли сохраненную копию напрямую
	boardCopy := make(domainModel.Board, len(game.Board))
	for i := range game.Board {
		boardCopy[i] = append([]int(nil), game.Board[i]...)
	}
	game.Board = boardCopy
	m.games[game.ID] = game
	return nil
}

func (m *mockGameRepo) GetGameByID(_ context.Context, id uuid.UUID) (domainModel.Game, error) {
	if m.loadErr != nil {
		return domainModel.Game{}, m.loadErr
	}
	g, ok := m.games[id]
	if !ok {
		return domainModel.Game{}, storage.ErrGameNotFound
	}
	return g, nil
}

func (m *mockGameRepo) GetGamesByState(_ context.Context, state string) ([]domainModel.Game, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	var res []domainModel.Game
	for _, g := range m.games {
		if g.GameState == state {
			res = append(res, g)
		}
	}
	return res, nil
}

func (m *mockGameRepo) GetCurrentGame(_ context.Context, userID uuid.UUID) (domainModel.Game, error) {
	if m.loadErr != nil {
		return domainModel.Game{}, m.loadErr
	}
	for _, g := range m.games {
		isParticipant := g.HostID == userID || (g.OpponentID != nil && *g.OpponentID == userID)
		if isParticipant && (g.GameState == domainModel.StateWaiting || g.GameState == domainModel.StatePlaying) {
			return g, nil
		}
	}
	return domainModel.Game{}, storage.ErrGameNotFound
}

func TestAppService_CreateGame(t *testing.T) {
	ctx := context.Background()
	repo := newMockGameRepo()
	svc := NewAppService(service.NewGameService(), repo)
	hostID := uuid.New()

	t.Run("invalid mode returns ErrInvalidMode", func(t *testing.T) {
		_, err := svc.CreateGame(ctx, hostID, "battle_royale")
		if !errors.Is(err, ErrInvalidMode) {
			t.Fatalf("expected ErrInvalidMode, got %v", err)
		}
	})

	t.Run("VSplayer starts in waiting state", func(t *testing.T) {
		g, err := svc.CreateGame(ctx, hostID, domainModel.ModeVSPlayer)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g.GameState != domainModel.StateWaiting || g.OpponentID != nil || g.NextPlayerID != nil {
			t.Fatalf("unexpected VSplayer initial state: %+v", g)
		}
		if g.Symbols[hostID.String()] != domainModel.SymbolX {
			t.Fatalf("expected host symbol X, got %v", g.Symbols)
		}
	})

	t.Run("VScomputer starts in playing state with ComputerID opponent", func(t *testing.T) {
		g, err := svc.CreateGame(ctx, hostID, domainModel.ModeVSComputer)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if g.GameState != domainModel.StatePlaying {
			t.Fatalf("expected playing state, got %s", g.GameState)
		}
		if g.OpponentID == nil || *g.OpponentID != domainModel.ComputerID {
			t.Fatalf("expected ComputerID opponent, got %v", g.OpponentID)
		}
		if g.NextPlayerID == nil || *g.NextPlayerID != hostID {
			t.Fatalf("expected host as next player, got %v", g.NextPlayerID)
		}
	})
}

func TestAppService_JoinGameErrorsAndSuccess(t *testing.T) {
	ctx := context.Background()
	repo := newMockGameRepo()
	svc := NewAppService(service.NewGameService(), repo)

	hostID := uuid.New()
	oppID := uuid.New()
	thirdPlayerID := uuid.New()

	t.Run("non-existent game returns ErrGameNotFound", func(t *testing.T) {
		_, err := svc.JoinGame(ctx, uuid.New(), oppID)
		if !errors.Is(err, ErrGameNotFound) {
			t.Fatalf("expected ErrGameNotFound, got %v", err)
		}
	})

	t.Run("cannot join VScomputer game", func(t *testing.T) {
		compGame, _ := svc.CreateGame(ctx, hostID, domainModel.ModeVSComputer)
		_, err := svc.JoinGame(ctx, compGame.ID, oppID)
		if !errors.Is(err, ErrNotVSPlayer) {
			t.Fatalf("expected ErrNotVSPlayer, got %v", err)
		}
	})

	pvpGame, _ := svc.CreateGame(ctx, hostID, domainModel.ModeVSPlayer)

	t.Run("creator cannot join own game", func(t *testing.T) {
		_, err := svc.JoinGame(ctx, pvpGame.ID, hostID)
		if !errors.Is(err, ErrCreatorCannotJoin) {
			t.Fatalf("expected ErrCreatorCannotJoin, got %v", err)
		}
	})

	t.Run("opponent successfully joins waiting game", func(t *testing.T) {
		joined, err := svc.JoinGame(ctx, pvpGame.ID, oppID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if joined.GameState != domainModel.StatePlaying {
			t.Fatalf("expected state playing, got %s", joined.GameState)
		}
		if joined.OpponentID == nil || *joined.OpponentID != oppID {
			t.Fatalf("expected opponent %s, got %v", oppID, joined.OpponentID)
		}
		if joined.Symbols[oppID.String()] != domainModel.SymbolO {
			t.Fatalf("expected opponent symbol O, got %v", joined.Symbols)
		}
	})

	t.Run("third player cannot join already started game", func(t *testing.T) {
		_, err := svc.JoinGame(ctx, pvpGame.ID, thirdPlayerID)
		if !errors.Is(err, ErrNotWaiting) {
			t.Fatalf("expected ErrNotWaiting, got %v", err)
		}
	})
}

func TestAppService_ApplyMoveErrorsAndFlow(t *testing.T) {
	ctx := context.Background()
	repo := newMockGameRepo()
	svc := NewAppService(service.NewGameService(), repo)

	hostID := uuid.New()
	oppID := uuid.New()
	strangerID := uuid.New()

	t.Run("non-existent game returns ErrGameNotFound", func(t *testing.T) {
		_, err := svc.ApplyMove(ctx, uuid.New(), hostID, 0, 0)
		if !errors.Is(err, ErrGameNotFound) {
			t.Fatalf("expected ErrGameNotFound, got %v", err)
		}
	})

	waitingGame, _ := svc.CreateGame(ctx, hostID, domainModel.ModeVSPlayer)

	t.Run("move in waiting game returns ErrGameNotPlaying", func(t *testing.T) {
		_, err := svc.ApplyMove(ctx, waitingGame.ID, hostID, 0, 0)
		if !errors.Is(err, ErrGameNotPlaying) {
			t.Fatalf("expected ErrGameNotPlaying, got %v", err)
		}
	})

	_, _ = svc.JoinGame(ctx, waitingGame.ID, oppID)

	t.Run("stranger cannot move in game", func(t *testing.T) {
		_, err := svc.ApplyMove(ctx, waitingGame.ID, strangerID, 0, 0)
		if !errors.Is(err, ErrNotYourGame) {
			t.Fatalf("expected ErrNotYourGame, got %v", err)
		}
	})

	t.Run("opponent cannot move on host turn", func(t *testing.T) {
		_, err := svc.ApplyMove(ctx, waitingGame.ID, oppID, 0, 0)
		if !errors.Is(err, ErrNotYourTurn) {
			t.Fatalf("expected ErrNotYourTurn, got %v", err)
		}
	})

	t.Run("out of bounds coordinates return ErrCellOutOfBounds", func(t *testing.T) {
		coords := [][2]int{{-1, 0}, {0, -1}, {3, 0}, {0, 3}}
		for _, c := range coords {
			_, err := svc.ApplyMove(ctx, waitingGame.ID, hostID, c[0], c[1])
			if !errors.Is(err, ErrCellOutOfBounds) {
				t.Fatalf("coords %v: expected ErrCellOutOfBounds, got %v", c, err)
			}
		}
	})

	t.Run("occupied cell returns ErrCellOccupied", func(t *testing.T) {
		_, err := svc.ApplyMove(ctx, waitingGame.ID, hostID, 0, 0)
		if err != nil {
			t.Fatalf("valid move failed: %v", err)
		}
		// Ход перешел к oppID, пытаемся сходить в ту же клетку (0,0)
		_, err = svc.ApplyMove(ctx, waitingGame.ID, oppID, 0, 0)
		if !errors.Is(err, ErrCellOccupied) {
			t.Fatalf("expected ErrCellOccupied, got %v", err)
		}
	})

	t.Run("full game to host victory resets NextPlayerID", func(t *testing.T) {
		// В waitingGame уже стоит X на (0,0), ход за oppID
		_, _ = svc.ApplyMove(ctx, waitingGame.ID, oppID, 1, 0)       // O -> (1,0)
		_, _ = svc.ApplyMove(ctx, waitingGame.ID, hostID, 0, 1)      // X -> (0,1)
		_, _ = svc.ApplyMove(ctx, waitingGame.ID, oppID, 1, 1)       // O -> (1,1)
		fin, err := svc.ApplyMove(ctx, waitingGame.ID, hostID, 0, 2) // X -> (0,2) WIN
		if err != nil {
			t.Fatalf("winning move failed: %v", err)
		}
		if fin.GameState != domainModel.StateFinished {
			t.Fatalf("expected state %s, got %s", domainModel.StateFinished, fin.GameState)
		}
		if fin.WinnerID == nil || *fin.WinnerID != hostID {
			t.Fatalf("expected winner %s, got %v", hostID, fin.WinnerID)
		}
		if fin.NextPlayerID != nil {
			t.Fatalf("expected NextPlayerID to be nil after game end, got %v", fin.NextPlayerID)
		}
	})

	t.Run("full game to draw sets StateDraw and nil NextPlayerID", func(t *testing.T) {
		drawGame, _ := svc.CreateGame(ctx, hostID, domainModel.ModeVSPlayer)
		_, _ = svc.JoinGame(ctx, drawGame.ID, oppID)

		// Сценарий ничьей:
		// X O X
		// X O O
		// O X X
		moves := []struct {
			player   uuid.UUID
			row, col int
		}{
			{hostID, 0, 0}, // X
			{oppID, 0, 1},  // O
			{hostID, 0, 2}, // X
			{oppID, 1, 1},  // O
			{hostID, 1, 0}, // X
			{oppID, 1, 2},  // O
			{hostID, 2, 1}, // X
			{oppID, 2, 0},  // O
		}
		for _, m := range moves {
			if _, err := svc.ApplyMove(ctx, drawGame.ID, m.player, m.row, m.col); err != nil {
				t.Fatalf("move (%d,%d) failed: %v", m.row, m.col, err)
			}
		}

		last, err := svc.ApplyMove(ctx, drawGame.ID, hostID, 2, 2)
		if err != nil {
			t.Fatalf("final move failed: %v", err)
		}
		if last.GameState != domainModel.StateDraw {
			t.Fatalf("expected state %s, got %s", domainModel.StateDraw, last.GameState)
		}
		if last.WinnerID != nil || last.NextPlayerID != nil {
			t.Fatalf("expected nil WinnerID and NextPlayerID on draw, got winner=%v next=%v", last.WinnerID, last.NextPlayerID)
		}
	})
}
