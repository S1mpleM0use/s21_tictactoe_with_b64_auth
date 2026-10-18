package app

import "errors"

var (
	ErrInvalidMove    = errors.New("invalid move")
	ErrInvalidMode    = errors.New("invalid game mode")
	ErrGameNotFound   = errors.New("game not found")
	ErrGameNotPlaying = errors.New("game is not active")

	ErrNotVSPlayer       = errors.New("game is not VSplayer mode")
	ErrNotWaiting        = errors.New("game is not waiting for players")
	ErrCreatorCannotJoin = errors.New("creator cannot join own game")

	ErrNotYourGame = errors.New("not your game")
	ErrNotYourTurn = errors.New("not your turn")

	ErrCellOutOfBounds = errors.New("cell out of bounds")
	ErrCellOccupied    = errors.New("cell occupied")

	ErrLoginRequired      = errors.New("login is required")
	ErrLoginTooLong       = errors.New("login must be at most 20 characters")
	ErrPasswordTooShort   = errors.New("password too short: must be 4+ characters")
	ErrLoginAlreadyTaken  = errors.New("login already taken")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")

	ErrAuthNotBasic  = errors.New("AUTH: Not 'Basic' scheme")
	ErrAuthNotBase64 = errors.New("AUTH: Encoding not base64")
	ErrAuthFormat    = errors.New("AUTH: login:password format failure")
)
