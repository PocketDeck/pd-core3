package game

// implement Game interface
type UnoGame struct {
	
}

func NewUnoGame() *UnoGame {
	return &UnoGame{}
}

func (u *UnoGame) HandleAction(userID int, payload []byte) {
	
}

func (u *UnoGame) State(userID int) any {
	return nil
}
