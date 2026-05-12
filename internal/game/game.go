package game

import "encoding/json"

type GameMessage struct {
	Target int              // -1 = broadcast, otherwise player ID
	Data   json.RawMessage
}

type Game interface {
	HandleAction(playerID int, payload []byte) []GameMessage
	State(playerID int) any
	Type() GameType
	Start(playerIDs []int) []GameMessage
}

type GameType string

const (
	GameUno GameType = "uno"
)

func NewGame(gameType GameType, config map[string]interface{}) Game {
	switch gameType {
	case GameUno:
		return NewUnoGame(config)
	default:
		return nil
	}
}

func IsValidGameType(gameType GameType) bool {
	switch gameType {
	case GameUno:
		return true
	default:
		return false
	}
}

func marshalMsg(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return json.RawMessage(data)
}
