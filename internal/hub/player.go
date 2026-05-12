package hub

type Player struct {
	ID       int
	Name     string
	Points   int
	IsActive bool
	Room     *Room
	User     *User
}

func NewPlayer(id int, name string) *Player {
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
