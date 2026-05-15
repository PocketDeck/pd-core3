package hub

import "PocketDeck/pd-core3/internal/game"

type Player struct {
	ID       game.PID
	Name     string
	Points   int
	IsActive bool
	Room     *Room
	User     *User
}

func NewPlayer(id game.PID, name string) *Player {
	return &Player{
		ID:       id,
		Name:     name,
		Points:   0,
		IsActive: false,
		Room:     nil,
		User:     nil,
	}
}

func (p *Player) Send(msg []byte) bool {
	if p.User == nil {
		return false
	}
	return p.User.TrySend(msg)
}

func (p *Player) AddPoints(amount int) {
	p.Points += amount
}
