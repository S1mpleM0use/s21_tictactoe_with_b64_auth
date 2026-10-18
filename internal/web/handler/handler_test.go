package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tictactoe/internal/app"
	domainModel "tictactoe/internal/domain/model"
	"tictactoe/internal/domain/service"
	"tictactoe/internal/infrastructure/datasource/storage"
	"tictactoe/internal/infrastructure/datasource/user"

	"github.com/google/uuid"
)

// --- In-memory fakes для тестирования полного пайплайна Handler -> App -> Repo ---

type fakeUserRepo struct {
	byID    map[uuid.UUID]domainModel.User
	byLogin map[string]domainModel.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{
		byID:    make(map[uuid.UUID]domainModel.User),
		byLogin: make(map[string]domainModel.User),
	}
}

func (f *fakeUserRepo) SaveUser(_ context.Context, u domainModel.User) error {
	if _, exists := f.byLogin[u.Login]; exists {
		return user.ErrLoginAlreadyTaken
	}
	f.byID[u.ID] = u
	f.byLogin[u.Login] = u
	return nil
}

func (f *fakeUserRepo) GetUserByLogin(_ context.Context, login string) (domainModel.User, error) {
	u, ok := f.byLogin[login]
	if !ok {
		return domainModel.User{}, user.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) GetUserByID(_ context.Context, id uuid.UUID) (domainModel.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return domainModel.User{}, user.ErrUserNotFound
	}
	return u, nil
}

type fakeGameRepo struct {
	games map[uuid.UUID]domainModel.Game
}

func newFakeGameRepo() *fakeGameRepo {
	return &fakeGameRepo{games: make(map[uuid.UUID]domainModel.Game)}
}

func (f *fakeGameRepo) SaveGame(_ context.Context, g domainModel.Game) error {
	f.games[g.ID] = g
	return nil
}

func (f *fakeGameRepo) GetGameByID(_ context.Context, id uuid.UUID) (domainModel.Game, error) {
	g, ok := f.games[id]
	if !ok {
		return domainModel.Game{}, storage.ErrGameNotFound
	}
	return g, nil
}

func (f *fakeGameRepo) GetGamesByState(_ context.Context, state string) ([]domainModel.Game, error) {
	var res []domainModel.Game
	for _, g := range f.games {
		if g.GameState == state {
			res = append(res, g)
		}
	}
	return res, nil
}

func (f *fakeGameRepo) GetCurrentGame(_ context.Context, userID uuid.UUID) (domainModel.Game, error) {
	for _, g := range f.games {
		if (g.HostID == userID || (g.OpponentID != nil && *g.OpponentID == userID)) &&
			(g.GameState == domainModel.StateWaiting || g.GameState == domainModel.StatePlaying) {
			return g, nil
		}
	}
	return domainModel.Game{}, storage.ErrGameNotFound
}

func setupTestMux() *http.ServeMux {
	uRepo := newFakeUserRepo()
	gRepo := newFakeGameRepo()
	userSvc := app.NewUserService(uRepo)
	authSvc := app.NewAuthService(userSvc)
	appSvc := app.NewAppService(service.NewGameService(), gRepo)
	authn := NewUserAuthenticator(authSvc)
	h := NewHandler(appSvc, authSvc, userSvc, authn)
	return h.Routes()
}

func makeBasicAuth(login, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(login+":"+pass))
}

func TestHandler_RegisterAndLogin(t *testing.T) {
	mux := setupTestMux()

	t.Run("POST /user invalid JSON cases return 400 and >1MB returns 413", func(t *testing.T) {
		badPayloads := []string{
			``,   // пустое тело
			`[]`, // не JSON-объект
			`{"login":"user1","password":"123","unknown_field":true}`, // неизвестное поле
			`{"login":"user1","password":"1234"}{"extra":1}`,          // два JSON-объекта подряд
			`{"login":"","password":"1234"}`,                          // пустой логин
			`{"login":"user1","password":"123"}`,                      // короткий пароль
		}
		for _, body := range badPayloads {
			req := httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("payload %q: expected 400, got %d", body, rec.Code)
			}
		}

		// Превышение лимита 1 MiB
		hugeBody := `{"login":"a","password":"` + strings.Repeat("x", maxRequestBodyBytes+100) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(hugeBody))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413 RequestEntityTooLarge, got %d", rec.Code)
		}
	})

	t.Run("POST /user 201 Created and 409 Conflict on duplicate", func(t *testing.T) {
		body := `{"login":"alice","password":"password123"}`
		req := httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		// Дубликат
		reqDup := httptest.NewRequest(http.MethodPost, "/user", strings.NewReader(body))
		recDup := httptest.NewRecorder()
		mux.ServeHTTP(recDup, reqDup)
		if recDup.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d", recDup.Code)
		}
	})

	t.Run("POST /user/login 200 OK and 401 Unauthorized on bad credentials", func(t *testing.T) {
		reqOK := httptest.NewRequest(http.MethodPost, "/user/login", nil)
		reqOK.Header.Set("Authorization", makeBasicAuth("alice", "password123"))
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, reqOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recOK.Code)
		}

		reqBad := httptest.NewRequest(http.MethodPost, "/user/login", nil)
		reqBad.Header.Set("Authorization", makeBasicAuth("alice", "wrongpass"))
		recBad := httptest.NewRecorder()
		mux.ServeHTTP(recBad, reqBad)
		if recBad.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", recBad.Code)
		}
		if recBad.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("expected WWW-Authenticate header on 401 response")
		}
	})
}

func TestHandler_ProtectedEndpointsAndGameErrors(t *testing.T) {
	mux := setupTestMux()

	// Регистрируем двух игроков: alice и bob
	for _, u := range []string{"alice", "bob"} {
		req := httptest.NewRequest(http.MethodPost, "/user",
			strings.NewReader(`{"login":"`+u+`","password":"pass123"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %s failed: %d", u, rec.Code)
		}
	}
	aliceAuth := makeBasicAuth("alice", "pass123")
	bobAuth := makeBasicAuth("bob", "pass123")

	t.Run("unauthenticated access to protected routes returns 401", func(t *testing.T) {
		protected := []struct {
			method, path string
		}{
			{http.MethodPost, "/game"},
			{http.MethodGet, "/game/list"},
			{http.MethodGet, "/game/current"},
			{http.MethodGet, "/user/" + uuid.NewString()},
			{http.MethodPost, "/game/" + uuid.NewString() + "/join"},
			{http.MethodPost, "/game/" + uuid.NewString() + "/move"},
		}
		for _, ep := range protected {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: expected 401, got %d", ep.method, ep.path, rec.Code)
			}
		}
	})

	t.Run("GET /game/current when no active game returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/game/current", nil)
		req.Header.Set("Authorization", aliceAuth)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 NotFound, got %d", rec.Code)
		}
	})

	t.Run("POST /game with invalid mode returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/game", strings.NewReader(`{"mode":"unknown"}`))
		req.Header.Set("Authorization", aliceAuth)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 BadRequest, got %d", rec.Code)
		}
	})

	// Создаем игру VSplayer от имени alice
	var createdGame struct {
		ID     uuid.UUID `json:"id"`
		HostID uuid.UUID `json:"host_id"`
	}
	{
		req := httptest.NewRequest(http.MethodPost, "/game", strings.NewReader(`{"mode":"VSplayer"}`))
		req.Header.Set("Authorization", aliceAuth)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", rec.Code)
		}
		_ = json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&createdGame)
	}

	t.Run("POST /game/{gameID}/join error codes (400, 403, 404, 409)", func(t *testing.T) {
		// 400 Невалидный UUID
		reqBadUUID := httptest.NewRequest(http.MethodPost, "/game/not-a-uuid/join", nil)
		reqBadUUID.Header.Set("Authorization", bobAuth)
		recBadUUID := httptest.NewRecorder()
		mux.ServeHTTP(recBadUUID, reqBadUUID)
		if recBadUUID.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid gameID, got %d", recBadUUID.Code)
		}

		// 404 Игра не найдена
		req404 := httptest.NewRequest(http.MethodPost, "/game/"+uuid.NewString()+"/join", nil)
		req404.Header.Set("Authorization", bobAuth)
		rec404 := httptest.NewRecorder()
		mux.ServeHTTP(rec404, req404)
		if rec404.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec404.Code)
		}

		// 403 Создатель пытается войти в свою же игру
		req403 := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/join", nil)
		req403.Header.Set("Authorization", aliceAuth)
		rec403 := httptest.NewRecorder()
		mux.ServeHTTP(rec403, req403)
		if rec403.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec403.Code)
		}

		// 200 Bob успешно присоединяется
		reqOK := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/join", nil)
		reqOK.Header.Set("Authorization", bobAuth)
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, reqOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("expected 200 OK on join, got %d", recOK.Code)
		}

		// 409 Повторное присоединение к уже начатой игре
		req409 := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/join", nil)
		req409.Header.Set("Authorization", bobAuth)
		rec409 := httptest.NewRecorder()
		mux.ServeHTTP(rec409, req409)
		if rec409.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d", rec409.Code)
		}
	})

	t.Run("POST /game/{gameID}/move error codes (400, 409) and valid move", func(t *testing.T) {
		// 409 Bob пытается сходить не в свой ход
		reqNotTurn := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/move",
			strings.NewReader(`{"row":0,"column":0}`))
		reqNotTurn.Header.Set("Authorization", bobAuth)
		recNotTurn := httptest.NewRecorder()
		mux.ServeHTTP(recNotTurn, reqNotTurn)
		if recNotTurn.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for wrong turn, got %d", recNotTurn.Code)
		}

		// 400 Alice ходит за пределы доски
		reqOOB := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/move",
			strings.NewReader(`{"row":5,"column":0}`))
		reqOOB.Header.Set("Authorization", aliceAuth)
		recOOB := httptest.NewRecorder()
		mux.ServeHTTP(recOOB, reqOOB)
		if recOOB.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 BadRequest for out-of-bounds move, got %d", recOOB.Code)
		}

		// 200 Alice делает валидный ход
		reqMove := httptest.NewRequest(http.MethodPost, "/game/"+createdGame.ID.String()+"/move",
			strings.NewReader(`{"row":0,"column":0}`))
		reqMove.Header.Set("Authorization", aliceAuth)
		recMove := httptest.NewRecorder()
		mux.ServeHTTP(recMove, reqMove)
		if recMove.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for valid move, got %d", recMove.Code)
		}
	})

	t.Run("GET /user/{userID} returns 200 without password, 400 on bad UUID, 404 on unknown", func(t *testing.T) {
		reqOK := httptest.NewRequest(http.MethodGet, "/user/"+createdGame.HostID.String(), nil)
		reqOK.Header.Set("Authorization", aliceAuth)
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, reqOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recOK.Code)
		}
		if strings.Contains(recOK.Body.String(), "password") || strings.Contains(recOK.Body.String(), "pass123") {
			t.Fatalf("user response leaked password field: %s", recOK.Body.String())
		}

		req400 := httptest.NewRequest(http.MethodGet, "/user/invalid-uuid", nil)
		req400.Header.Set("Authorization", aliceAuth)
		rec400 := httptest.NewRecorder()
		mux.ServeHTTP(rec400, req400)
		if rec400.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec400.Code)
		}

		req404 := httptest.NewRequest(http.MethodGet, "/user/"+uuid.NewString(), nil)
		req404.Header.Set("Authorization", aliceAuth)
		rec404 := httptest.NewRecorder()
		mux.ServeHTTP(rec404, req404)
		if rec404.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec404.Code)
		}
	})
}
