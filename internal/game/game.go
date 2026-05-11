package game

import "encoding/json"

type GameMessage struct {
	Target string          // "" = broadcast, otherwise player name
	Data   json.RawMessage
}

type Game interface {
	HandleAction(playerName string, payload []byte) []GameMessage
	State(playerName string) any
	Type() GameType
	Start(players []string) []GameMessage
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
