package hub

import (
	"encoding/json"

	"PocketDeck/pd-core3/internal/game"
	"sync"
)

type Room struct {
	ID      string
	players map[string]*Player
	users   map[int]*User
	game    game.Game

	mu     sync.RWMutex
	userID int
}

func NewRoom(id string, g game.Game) *Room {
	return &Room{
		ID:      id,
		game:    g,
		players: make(map[string]*Player),
		users:   make(map[int]*User),
	}
}

func (r *Room) GetPlayer(name string) *Player {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.players[name]
}

func (r *Room) GetUser(id int) *User {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.users[id]
}

func (r *Room) AddPlayer(name string) *Player {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := NewPlayer(name)
	player.Room = r
	r.players[name] = player
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

func (r *Room) ConnectUserToPlayer(user *User, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	player, ok := r.players[name]
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
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.game == nil {
		return ""
	}

	return r.game.Type()
}

func (r *Room) HandleAction(playerName string, payload []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.game == nil {
		return
	}

	r.game.HandleAction(playerName, payload)
}

func (r *Room) GameState(playerName string) any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.game == nil {
		return nil
	}

	return r.game.State(playerName)
}

func (r *Room) Broadcast(msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, u := range r.users {
		u.TrySend(msg)
	}
}

func (r *Room) BroadcastOthers(excludePlayerName string, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, p := range r.players {
		if name != excludePlayerName && p.User != nil {
			p.User.TrySend(msg)
		}
	}
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
