package handler

import (
	"errors"
	"tictactoe/internal/app"
)

func isAuthenticationError(err error) bool {
	return errors.Is(err, app.ErrInvalidCredentials) ||
		errors.Is(err, app.ErrAuthNotBasic) ||
		errors.Is(err, app.ErrAuthNotBase64) ||
		errors.Is(err, app.ErrAuthFormat)
}
