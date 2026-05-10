package game

type UnoGame struct {
}

func NewUnoGame() *UnoGame {
	return &UnoGame{}
}

func (u *UnoGame) HandleAction(playerName string, payload []byte) {
}

func (u *UnoGame) State(playerName string) any {
	return nil
}
