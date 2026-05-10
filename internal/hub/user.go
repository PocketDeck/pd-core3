package hub

import (
	"sync/atomic"
)

type User struct {
	ID     int
	Player *Player
	ready  atomic.Bool
	Send   chan []byte
}

func NewUser(id int, send chan []byte) *User {
	return &User{
		ID:    id,
		ready: atomic.Bool{},
		Send:  send,
	}
}

func (u *User) IsReady() bool {
	return u.ready.Load()
}

func (u *User) SetReady(v bool) {
	u.ready.Store(v)
}

func (u *User) TrySend(msg []byte) bool {
	select {
	case u.Send <- msg:
		return true
	default:
		return false
	}
}
