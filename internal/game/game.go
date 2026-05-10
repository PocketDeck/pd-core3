package game

type Game interface {
	HandleAction(playerName string, payload []byte)
	State(playerName string) any
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
