package game

import (
	"encoding/json"
	"math"
	"sort"
)

type PID uint

const (
	BroadcastPID             PID = math.MaxUint
	BroadcastExceptCurrentPID PID = math.MaxUint - 1
)

type GameMessage struct {
	Target PID
	Data   json.RawMessage
}

type Game interface {
	HandleAction(playerID PID, payload []byte) []GameMessage
	State(playerID PID) any
	Start(playerIDs []PID) []GameMessage
	Type() GameType
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

func SquashIDs(ids []PID) []PID {
	if len(ids) == 0 {
		return nil
	}

	sorted := make([]PID, len(ids))
	copy(sorted, ids)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	mapping := make(map[PID]PID, len(ids))
	for i, id := range sorted {
		mapping[id] = PID(i)
	}

	result := make([]PID, len(ids))
	for i, id := range ids {
		result[i] = mapping[id]
	}
	return result
}
