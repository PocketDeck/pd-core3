package hub

import (
	"encoding/json"

	"PocketDeck/pd-core3/internal/game"
	"sync"
)

type Room struct {
	ID      string
	players map[game.PID]*Player
	users   map[int]*User
	game    game.Game

	gameType   game.GameType
	gameConfig map[string]interface{}

	mu           sync.RWMutex
	nextPlayerID game.PID
	userID       int
	playerOrder  []game.PID
}

func NewRoom(id string, gameType game.GameType, config map[string]interface{}) *Room {
	return &Room{
		ID:         id,
		gameType:   gameType,
		gameConfig: config,
		players:    make(map[game.PID]*Player),
		users:      make(map[int]*User),
		playerOrder: make([]game.PID, 0),
	}
}

func (r *Room) GetPlayer(name string) *Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.players {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (r *Room) GetPlayerByPID(id game.PID) *Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.players[id]
}

func (r *Room) GetUser(id int) *User {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.users[id]
}

func (r *Room) AddPlayer(name string) *Player {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := NewPlayer(r.nextPlayerID, name)
	player.Room = r
	r.players[player.ID] = player
	r.playerOrder = append(r.playerOrder, player.ID)
	r.nextPlayerID++
	return player
}

func (r *Room) AddUser(sendChan chan []byte) *User {
	r.mu.Lock()
	defer r.mu.Unlock()

	user := NewUser(r.userID, sendChan)
	r.users[r.userID] = user
	r.userID++
	return user
}

func (r *Room) ConnectUserToPlayer(user *User, id game.PID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.players[id]
	if !ok {
		return false
	}

	if player.User != nil {
		player.User.Player = nil
	}

	user.Player = player
	player.User = user
	player.IsActive = true
	return true
}

func (r *Room) RemoveUser(userID int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, ok := r.users[userID]
	if !ok {
		return
	}

	if user.Player != nil {
		user.Player.User = nil
		user.Player.IsActive = false
		user.Player = nil
	}

	delete(r.users, userID)
}

func (r *Room) RemovePlayer(id game.PID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.players, id)
	for i, pid := range r.playerOrder {
		if pid == id {
			r.playerOrder = append(r.playerOrder[:i], r.playerOrder[i+1:]...)
			break
		}
	}
}

func (r *Room) AllReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.players {
		if p.User != nil && !p.User.IsReady() {
			return false
		}
	}
	return true
}

func (r *Room) GameType() game.GameType {
	return r.gameType
}

func (r *Room) StartGame(playerIDs []game.PID) {
	r.mu.Lock()
	if r.game != nil {
		r.mu.Unlock()
		return
	}

	squashed := game.SquashIDs(playerIDs)

	oldToNew := make(map[game.PID]game.PID, len(playerIDs))
	for i, old := range playerIDs {
		oldToNew[old] = squashed[i]
	}

	newPlayers := make(map[game.PID]*Player, len(r.players))
	for _, p := range r.players {
		newID := oldToNew[p.ID]
		p.ID = newID
		newPlayers[newID] = p
	}
	r.players = newPlayers
	r.playerOrder = make([]game.PID, len(squashed))
	copy(r.playerOrder, squashed)

	r.game = game.NewGame(r.gameType, r.gameConfig)
	r.mu.Unlock()

	if r.game == nil {
		return
	}
	messages := r.game.Start(squashed)
	r.processMessages(messages, game.BroadcastPID)
}

func (r *Room) HandleAction(playerID game.PID, payload []byte) {
	if r.game == nil {
		return
	}
	messages := r.game.HandleAction(playerID, payload)
	r.processMessages(messages, playerID)
}

func (r *Room) processMessages(messages []game.GameMessage, excludePID game.PID) {
	for _, m := range messages {
		switch m.Target {
		case game.BroadcastPID:
			r.Broadcast([]byte(m.Data))
		case game.BroadcastExceptCurrentPID:
			r.BroadcastOthers(excludePID, []byte(m.Data))
		default:
			r.sendToPlayer(m.Target, []byte(m.Data))
		}
	}
}

func (r *Room) GameState(playerID game.PID) any {
	if r.game == nil {
		return nil
	}
	return r.game.State(playerID)
}

func (r *Room) GetPlayerIDs() []game.PID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]game.PID, len(r.playerOrder))
	copy(ids, r.playerOrder)
	return ids
}

func (r *Room) Broadcast(msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		u.TrySend(msg)
	}
}

func (r *Room) sendToPlayer(playerID game.PID, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.players {
		if p.ID == playerID && p.User != nil {
			p.User.TrySend(msg)
			return
		}
	}
}

func (r *Room) BroadcastOthers(excludePlayerID game.PID, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.players {
		if p.ID != excludePlayerID && p.User != nil {
			p.User.TrySend(msg)
		}
	}
}

func (r *Room) ContainsPlayer(id game.PID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.players[id]
	return ok
}

func (r *Room) PlayerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.players)
}

func (r *Room) GameStarted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.game != nil
}

func (r *Room) SetGame(g game.Game) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.game = g
}

func (r *Room) GetPlayers() []*Player {
	r.mu.RLock()
	defer r.mu.RUnlock()

	players := make([]*Player, 0, len(r.players))
	for _, p := range r.players {
		players = append(players, p)
	}
	return players
}

func (r *Room) BroadcastPlayerUpdate() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	playerList := make([]map[string]interface{}, 0, len(r.players))
	for _, p := range r.players {
		playerList = append(playerList, map[string]interface{}{
			"id":     p.ID,
			"name":   p.Name,
			"points": p.Points,
			"active": p.IsActive,
			"ready":  p.User != nil && p.User.IsReady(),
		})
	}

	updateMsg, _ := json.Marshal(map[string]interface{}{
		"action":  "players",
		"players": playerList,
	})
	r.Broadcast(updateMsg)
}
