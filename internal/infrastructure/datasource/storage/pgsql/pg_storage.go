package pgsql

import (
	"context"
	"errors"
	"fmt"
	"log"
	infraModel "tictactoe/internal/infrastructure/datasource/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/google/uuid"
)

type GameStorage struct {
	pool *pgxpool.Pool
}

func NewGameStorage(cfg Config) (*GameStorage, error) {
	pool, err := pgxpool.New(context.Background(), cfg.FormConnString())
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &GameStorage{pool: pool}, nil
}

func (s *GameStorage) Close() {
	if s != nil && s.pool != nil {
		log.Println("Closing PostgreSQL connection pool...")
		s.pool.Close()
	}
}

func Migrate(ctx context.Context, pool *GameStorage) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
    id                 uuid PRIMARY KEY,
    login              varchar(20) NOT NULL,
    password_hash      varchar(64) NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS users_login_unique ON users (login);

CREATE TABLE IF NOT EXISTS games (
    id         uuid PRIMARY KEY,
    board      varchar NOT NULL,
	state      varchar NOT NULL,
	mode       varchar NOT NULL,
    host_id    uuid NOT NULL REFERENCES users(id),
    opp_id     uuid REFERENCES users(id),
	next_id    uuid REFERENCES users(id),
	symbols    jsonb NOT NULL DEFAULT '{}'::jsonb,
	winner_id  uuid REFERENCES users(id),
	created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO users (id, login, password_hash)
VALUES (
  '00000000-0000-0000-0000-000000000001',
  'computer',
  '!'         -- to prevent login attempts from bot's account       
)
ON CONFLICT (id) DO NOTHING;

`

	if _, err := pool.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("postgres migrate: %w", err)
	}
	return nil
}

func (s *GameStorage) StoreGame(ctx context.Context, e infraModel.GameDTO) error {
	const query = `
	INSERT INTO games (id, board, state, mode, host_id, opp_id, next_id, symbols, winner_id)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT (id) DO UPDATE SET
    board     = EXCLUDED.board,
    state     = EXCLUDED.state,
    mode      = EXCLUDED.mode,
    host_id   = EXCLUDED.host_id,
    opp_id    = EXCLUDED.opp_id,
    next_id   = EXCLUDED.next_id,
    symbols   = EXCLUDED.symbols,
    winner_id = EXCLUDED.winner_id,
	updated_at = now();
`

	_, err := s.pool.Exec(ctx, query,
		e.ID,
		e.Board,
		e.GameState,
		e.Mode,
		e.HostID,
		e.OpponentID,
		e.NextPlayerID,
		e.Symbols,
		e.WinnerID,
	)
	return err
}

func (s *GameStorage) LoadGameByID(ctx context.Context, id uuid.UUID) (infraModel.GameDTO, error) {
	var game infraModel.GameDTO

	const getGameQuery = `
	SELECT id, board, state, mode, host_id, opp_id, next_id, symbols, winner_id
	FROM games WHERE id = $1`

	err := s.pool.QueryRow(ctx, getGameQuery, id).Scan(
		&game.ID,
		&game.Board,
		&game.GameState,
		&game.Mode,
		&game.HostID,
		&game.OpponentID,
		&game.NextPlayerID,
		&game.Symbols,
		&game.WinnerID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return infraModel.GameDTO{}, fmt.Errorf("%w: %s", ErrGameNotFound, id)
		}
		return infraModel.GameDTO{}, fmt.Errorf("DB: loading game by ID failed: %w", err)
	}
	return game, nil
}

func (s *GameStorage) LoadGamesByState(ctx context.Context, state string) ([]infraModel.GameDTO, error) {
	const getGameQuery = `
	SELECT id, board, state, mode, host_id, opp_id, next_id, symbols, winner_id
	FROM games WHERE state = $1`

	rows, err := s.pool.Query(ctx, getGameQuery, state)
	if err != nil {
		return nil, fmt.Errorf("query games by state %q: %w", state, err)
	}
	defer rows.Close()

	var games []infraModel.GameDTO
	for rows.Next() {
		var g infraModel.GameDTO
		if err := rows.Scan(
			&g.ID,
			&g.Board,
			&g.GameState,
			&g.Mode,
			&g.HostID,
			&g.OpponentID,
			&g.NextPlayerID,
			&g.Symbols,
			&g.WinnerID,
		); err != nil {
			return nil, fmt.Errorf("scan game: %w", err)
		}
		games = append(games, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate games: %w", err)
	}
	return games, nil
}

func (s *GameStorage) LoadCurrentGame(ctx context.Context, userID uuid.UUID) (infraModel.GameDTO, error) {
	var game infraModel.GameDTO

	const loadCurrentGameQuery = `
	SELECT id, board, state, mode, host_id, opp_id, next_id, symbols, winner_id
	FROM games
	WHERE (host_id = $1 OR opp_id = $1)
	AND state IN ('waiting', 'playing')
	ORDER BY updated_at DESC
	LIMIT 1;
	`

	err := s.pool.QueryRow(ctx, loadCurrentGameQuery, userID).Scan(
		&game.ID,
		&game.Board,
		&game.GameState,
		&game.Mode,
		&game.HostID,
		&game.OpponentID,
		&game.NextPlayerID,
		&game.Symbols,
		&game.WinnerID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return infraModel.GameDTO{}, ErrGameNotFound
		}
		return infraModel.GameDTO{}, fmt.Errorf("DB: loading current games failed: %w", err)
	}
	return game, nil

}

func (s *GameStorage) SaveUser(ctx context.Context, user infraModel.UserDTO) error {
	const query = `
	INSERT INTO users (id, login, password_hash) VALUES ($1, $2, $3)
	`
	if _, err := s.pool.Exec(ctx, query, user.ID, user.Login, user.PasswordHash); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: %s", ErrLoginAlreadyTaken, user.Login)
		}
		return fmt.Errorf("DB: save user: %w", err)
	}
	return nil
}

const userColumns = `id, login, password_hash`

func (s *GameStorage) GetUserByLogin(ctx context.Context, login string) (infraModel.UserDTO, error) {
	var user infraModel.UserDTO
	err := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE login = $1`, userColumns), login).
		Scan(&user.ID, &user.Login, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return infraModel.UserDTO{}, fmt.Errorf("%w: user %q", ErrUserNotFound, login)
		}
		return infraModel.UserDTO{}, fmt.Errorf("Get user by login failed: %w", err)
	}
	return user, nil
}

func (s *GameStorage) GetUserByID(ctx context.Context, id uuid.UUID) (infraModel.UserDTO, error) {
	var user infraModel.UserDTO
	err := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM users WHERE id = $1::uuid`, userColumns), id).
		Scan(&user.ID, &user.Login, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return infraModel.UserDTO{}, fmt.Errorf("%w: user %q", ErrUserNotFound, id)
		}
		return infraModel.UserDTO{}, fmt.Errorf("get user by id failed: %w", err)
	}
	return user, nil
}
