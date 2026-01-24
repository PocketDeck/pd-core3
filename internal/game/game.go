package game

type Game interface {
	HandleAction(userID int, payload []byte)
	State(userID int) any
}

type GameType string

const (
	GameUno GameType = "uno"
)

func NewGame(gameType GameType) Game {
	switch gameType {
	case GameUno:
		return NewUnoGame()
	default:
		return nil
	}
}
