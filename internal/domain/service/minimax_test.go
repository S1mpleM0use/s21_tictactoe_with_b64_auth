package service

import (
	"testing"
	domainModel "tictactoe/internal/domain/model"

	"github.com/google/uuid"
)

func TestMinimaxScores(t *testing.T) {
	// Инициализируем пустой сервис (репозиторий для минимакса не нужен)
	svc := &gameService{}
	hostID := uuid.New()
	compID := domainModel.ComputerID

	tests := []struct {
		name      string
		board     domainModel.Board
		isMax     bool
		wantScore int // Положительный - бот выигрывает, отрицательный - бот проигрывает
	}{
		{
			name: "Computer (Circle) is about to win",
			board: domainModel.Board{
				{domainModel.Circle, domainModel.Circle, domainModel.Empty},
				{domainModel.Cross, domainModel.Cross, domainModel.Empty},
				{domainModel.Empty, domainModel.Empty, domainModel.Empty},
			},
			isMax:     true, // Ход бота
			wantScore: 9,    // 10 - depth(1) = 9
		},
		{
			name: "Player (Cross) is about to win, Computer must block",
			board: domainModel.Board{
				{domainModel.Cross, domainModel.Cross, domainModel.Empty},
				{domainModel.Circle, domainModel.Empty, domainModel.Empty},
				{domainModel.Empty, domainModel.Empty, domainModel.Empty},
			},
			isMax:     false, // Симулируем ход оппонента в ветке минимакса
			wantScore: -9,    // Бот видит, что игрок выиграет: -10 + depth(1) = -9
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := domainModel.Game{
				Board:      tt.board,
				HostID:     hostID,
				OpponentID: &compID,
			}

			score := svc.minimax(game, 0, tt.isMax)

			// Проверяем направление оценки (больше 0 для победы ПК, меньше 0 для победы игрока)
			if (tt.wantScore > 0 && score <= 0) || (tt.wantScore < 0 && score >= 0) {
				t.Errorf("Expected score around %d, got %d", tt.wantScore, score)
			}
		})
	}
}
