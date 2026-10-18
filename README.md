# completed by zhindasp / SimpleMouse

# HOW TO PLAY?
TEST A GAME: go test -v -race ./...

RUN A GAME: 
cd src ./start_psql.sh
go run cmd/app/main.go

register user: curl -X POST http://localhost:8080/user -H 'Content-Type: application/json' -d '{"login": "test_user_1", "password": "121212"}'

login: curl -X POST http://localhost:8080/user/login -u test_user_1:121212

create a game vs player: curl -X POST http://localhost:8080/game -u test_user_1:121212 -H 'Content-Type: application/json' -d '{"mode":"VSplayer"}'

vs computer: curl -X POST http://localhost:8080/game -u test_user_1:121212 -H 'Content-Type: application/json' -d '{"mode":"VScomputer"}'

join a game: curl -X POST http://localhost:8080/game/GAME_ID/join -u test_user_2:909090

make a move: curl -X POST http://localhost:8080/game/GAME_ID/move -u test_user_1:121212 -H 'Content-Type: application/json' -d '{"row": 0, "column": 0}'

get a current game: curl -X GET http://localhost:8080/game/current -u test_user_1:121212

get user info by ID: curl -X GET http://localhost:8080/user/PLAYER_ID -u test_user_1:121212

get current game for a user (a game with the most recent update time): curl -X GET http://localhost:8080/game/current -u test_user_1:121212


# What was the task?
In the previous project I was given a task to create a basic version of a game Tic Tac Toe with in-memory storage and no auth.
The goal was to practice student's understanding of **Clean Architecture**. 

The task here was:
1. To remove the in-memory structure, switch it to a **persisent storage** (in my case, PostgreSQL database)
2. To add a **User system** with authorization using base64 encryption
3. To add **Multiplayer capability** (creating a game against a bot or a player, joining game, getting list of available games)
