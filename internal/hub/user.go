package hub

import "sync/atomic"

type User struct {
	ID   int
	Name string

	ready atomic.Bool

	Send *chan []byte
}

func (u *User) IsReady() bool {
	return u.ready.Load()
}

func (u *User) SetReady(v bool) {
	u.ready.Store(v)
}

func (u *User) TrySend(msg []byte) bool {
	select {
	case *u.Send <- msg:
		return true
	default:
		return false
	}
}

