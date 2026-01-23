package hub

import (
	"PocketDeck/pd-core3/internal/game"
	"sync"
)

type Room struct {
	ID    string
	users map[int]*User
	game  game.Game

	mu    sync.Mutex
	idGen int
}

func NewRoom(id string, g game.Game) *Room {
	return &Room{
		ID:    id,
		game:  g,
		users: make(map[int]*User),
	}
}

func (r *Room) AddUser(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u.ID = r.idGen
	r.idGen++

	r.users[u.ID] = u
}

func (r *Room) RemoveUser(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.users, u.ID)
}

func (r *Room) AllReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.users {
		if !u.IsReady() {
			return false
		}
	}
	return true
}

func (r *Room) HandleAction(userID int, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.game == nil {
		return
	}

	r.game.HandleAction(userID, payload)
}

func (r *Room) BroadcastOthers(userID int, msg []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, u := range r.users {
		if id != userID {
			u.TrySend(msg)
		}
	}
}

func (r *Room) Broadcast(msg []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.users {
		u.TrySend(msg)
	}
}
