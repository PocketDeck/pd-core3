package game

import (
	"encoding/json"
	"testing"
)

func TestBuildDeck(t *testing.T) {
	deck := buildDeck()
	if len(deck) != 108 {
		t.Fatalf("Expected 108 cards, got %d", len(deck))
	}

	counts := map[CardKind]int{}
	for _, c := range deck {
		counts[c.Kind]++
	}
	if counts[KindNumber] != 76 {
		t.Errorf("Expected 76 number cards, got %d", counts[KindNumber])
	}
	if counts[KindSkip] != 8 {
		t.Errorf("Expected 8 skip cards, got %d", counts[KindSkip])
	}
	if counts[KindReverse] != 8 {
		t.Errorf("Expected 8 reverse cards, got %d", counts[KindReverse])
	}
	if counts[KindDraw2] != 8 {
		t.Errorf("Expected 8 draw2 cards, got %d", counts[KindDraw2])
	}
	if counts[KindWild] != 4 {
		t.Errorf("Expected 4 wild cards, got %d", counts[KindWild])
	}
	if counts[KindWildDraw4] != 4 {
		t.Errorf("Expected 4 wild draw4 cards, got %d", counts[KindWildDraw4])
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.CardsPerPlayer != 7 {
		t.Errorf("Expected 7 cards per player, got %d", cfg.CardsPerPlayer)
	}
	if cfg.PointsToWin != 500 {
		t.Errorf("Expected 500 points to win, got %d", cfg.PointsToWin)
	}
	if !cfg.PlayAfterDraw {
		t.Error("Expected PlayAfterDraw to be true")
	}
	if !cfg.AggregateDraws {
		t.Error("Expected AggregateDraws to be true")
	}
	if !cfg.BlackOnBlack {
		t.Error("Expected BlackOnBlack to be true")
	}
}

func TestNewUnoGameConfig(t *testing.T) {
	config := map[string]interface{}{
		"cardsPerPlayer": float64(5),
		"pointsToWin":    float64(300),
		"playAfterDraw":  false,
		"aggregateDraws": false,
		"blackOnBlack":   false,
	}
	g := NewUnoGame(config)
	if g.config.CardsPerPlayer != 5 {
		t.Errorf("Expected 5, got %d", g.config.CardsPerPlayer)
	}
	if g.config.PointsToWin != 300 {
		t.Errorf("Expected 300, got %d", g.config.PointsToWin)
	}
	if g.config.PlayAfterDraw {
		t.Error("Expected PlayAfterDraw false")
	}
	if g.config.AggregateDraws {
		t.Error("Expected AggregateDraws false")
	}
	if g.config.BlackOnBlack {
		t.Error("Expected BlackOnBlack false")
	}
}

func TestStart(t *testing.T) {
	g := NewUnoGame(nil)
	msgs := g.Start([]PID{0, 1})

	if msgs != nil {
		t.Error("Expected Start to return nil (state is fetched via status)")
	}
	if g.state != StatePlaying {
		t.Errorf("Expected state playing, got %v", g.state)
	}
	if len(g.playerIDs) != 2 {
		t.Errorf("Expected 2 players, got %d", len(g.playerIDs))
	}
	if len(g.hands[0]) != 7 {
		t.Errorf("Expected player 0 to have 7 cards, got %d", len(g.hands[0]))
	}
	if len(g.hands[1]) != 7 {
		t.Errorf("Expected player 1 to have 7 cards, got %d", len(g.hands[1]))
	}
	if len(g.discard) != 1 {
		t.Errorf("Expected 1 card on discard pile, got %d", len(g.discard))
	}
	if g.discard[0].Kind == KindWild || g.discard[0].Kind == KindWildDraw4 {
		t.Error("First discard should not be a wild card")
	}
	if g.currentTurn != 0 {
		t.Errorf("Expected current turn 0, got %d", g.currentTurn)
	}
	if g.direction != 1 {
		t.Errorf("Expected direction 1, got %d", g.direction)
	}
}

func TestPlayCardColorMatch(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindNumber, Value: 5}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	if len(msgs) == 0 {
		t.Fatal("Expected messages from play")
	}
	foundPlay := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "card_played" {
				foundPlay = true
				if data["player"] != float64(0) {
					t.Errorf("Expected player 0 in card_played, got %v", data["player"])
				}
			}
		}
	}
	if !foundPlay {
		t.Error("Expected card_played broadcast")
	}

	if len(g.discard) != 2 {
		t.Errorf("Expected 2 cards on discard pile, got %d", len(g.discard))
	}
	if g.discard[1].Color != ColorRed || g.discard[1].Value != 5 {
		t.Errorf("Expected red 5 on discard, got %v", g.discard[1])
	}
	if len(g.hands[0]) != 0 {
		t.Errorf("Expected player 0 to have 0 cards left, got %d", len(g.hands[0]))
	}
}

func TestPlayCardSymbolMatch(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	g.hands[0] = []Card{{Color: ColorBlue, Kind: KindSkip}}
	g.discard = []Card{{Color: ColorRed, Kind: KindSkip}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	if len(msgs) == 0 {
		t.Fatal("Expected messages")
	}
	foundPlay := false
	foundSkipped := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			switch data["action"] {
			case "card_played":
				foundPlay = true
			case "player_skipped":
				foundSkipped = true
			}
		}
	}
	if !foundPlay {
		t.Error("Expected card_played")
	}
	if !foundSkipped {
		t.Error("Expected player_skipped from skip card")
	}
}

func TestPlayCardNotYourTurn(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(1, payload)

	if len(msgs) != 1 {
		t.Fatalf("Expected 1 error message, got %d", len(msgs))
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "not_your_turn" {
		t.Errorf("Expected 'not_your_turn' error, got %v", data["error"])
	}
	if msgs[0].Target != 1 {
		t.Errorf("Error should be sent to player 1, got %d", msgs[0].Target)
	}
}

func TestPlayCardNotInHand(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 5}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 5,
	})
	msgs := g.HandleAction(0, payload)
	if len(msgs) == 0 {
		t.Fatal("Expected error message")
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "card_not_in_hand" {
		t.Errorf("Expected 'card_not_in_hand', got %v", data["error"])
	}
}

func TestPlayCardCannotPlay(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.hands[0] = []Card{{Color: ColorBlue, Kind: KindSkip}}
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)
	if len(msgs) == 0 {
		t.Fatal("Expected error message")
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "cannot_play_card" {
		t.Errorf("Expected 'cannot_play_card', got %v", data["error"])
	}
}

func TestPlayWildNeedsColor(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.hands[0] = []Card{{Color: ColorWild, Kind: KindWild}}
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)
	if len(msgs) == 0 {
		t.Fatal("Expected error message")
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "must_declare_color" {
		t.Errorf("Expected 'must_declare_color', got %v", data["error"])
	}
}

func TestPlayWildWithColor(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.hands[0] = []Card{{Color: ColorWild, Kind: KindWild}}
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"wildColor":  "blue",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	foundPlay := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "card_played" {
				foundPlay = true
			}
		}
	}
	if !foundPlay {
		t.Error("Expected card_played for wild card")
	}

	if len(g.discard) < 2 {
		t.Fatal("Expected at least 2 cards on discard")
	}
	if g.discard[1].Color != ColorBlue {
		t.Errorf("Expected wild to be declared blue, got %v", g.discard[1].Color)
	}
}

func TestDrawCard(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	initialCount := len(g.hands[0])

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "draw_card",
	})
	msgs := g.HandleAction(0, payload)

	if len(g.hands[0]) != initialCount+1 {
		t.Errorf("Expected %d cards after draw, got %d", initialCount+1, len(g.hands[0]))
	}

	foundDraw := false
	for _, m := range msgs {
		if m.Target == 0 {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "draw" {
				foundDraw = true
			}
		}
	}
	if !foundDraw {
		t.Error("Expected draw message to player 0")
	}
}

func TestDrawCardPenalty(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.drawCounter = 2
	initialCount := len(g.hands[0])

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "draw_card",
	})
	msgs := g.HandleAction(0, payload)

	if len(g.hands[0]) != initialCount+2 {
		t.Errorf("Expected %d cards after draw penalty, got %d", initialCount+2, len(g.hands[0]))
	}
	if g.drawCounter != 0 {
		t.Errorf("Expected draw counter to be 0, got %d", g.drawCounter)
	}

	foundDraw := false
	for _, m := range msgs {
		if m.Target == 0 {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "draw" {
				foundDraw = true
				cards := data["cards"].([]interface{})
				if len(cards) != 2 {
					t.Errorf("Expected 2 cards drawn, got %d", len(cards))
				}
			}
		}
	}
	if !foundDraw {
		t.Error("Expected draw message to player 0")
	}
}

func TestSkipEffect(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1, 2})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}

	g.hands[0] = []Card{{Color: ColorRed, Kind: KindSkip}}
	g.currentTurn = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	g.HandleAction(0, payload)

	if g.currentTurn != 2 {
		t.Errorf("After skip, expected turn at index 2 (player 2), got %d (playerID %d)", g.currentTurn, g.playerIDs[g.currentTurn])
	}
}

func TestReverseEffect(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1, 2})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindReverse}}
	g.currentTurn = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	g.HandleAction(0, payload)

	if g.direction != -1 {
		t.Errorf("Expected direction -1 after reverse, got %d", g.direction)
	}
}

func TestDraw2Effect(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindDraw2}}
	g.currentTurn = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	g.HandleAction(0, payload)

	if g.drawCounter != 2 {
		t.Errorf("Expected draw counter 2, got %d", g.drawCounter)
	}

	initialBob := len(g.hands[1])
	bobPayload, _ := json.Marshal(map[string]interface{}{
		"action": "draw_card",
	})
	g.HandleAction(1, bobPayload)
	if len(g.hands[1]) != initialBob+2 {
		t.Errorf("Expected player 1 to draw 2 penalty cards, got %d more", len(g.hands[1])-initialBob)
	}
}

func TestGameOver(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 5}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindNumber, Value: 5}}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	if g.state != StateFinished {
		t.Errorf("Expected StateFinished, got %v", g.state)
	}
	if g.winner != 0 {
		t.Errorf("Expected winner 0, got %d", g.winner)
	}

	foundOver := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "game_over" {
				foundOver = true
				if data["winner"] != float64(0) {
					t.Errorf("Expected winner 0 in game_over, got %v", data["winner"])
				}
			}
		}
	}
	if !foundOver {
		t.Error("Expected game_over broadcast")
	}
}

func TestUnoCall(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{
		{Color: ColorRed, Kind: KindNumber, Value: 5},
		{Color: ColorBlue, Kind: KindNumber, Value: 2},
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	if len(g.hands[0]) != 1 {
		t.Fatalf("Expected player 0 to have 1 card remaining after play, got %d", len(g.hands[0]))
	}

	foundUno := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "uno" {
				foundUno = true
				if data["player"] != float64(0) {
					t.Errorf("Expected uno player 0, got %v", data["player"])
				}
			}
		}
	}
	if !foundUno {
		t.Error("Expected uno broadcast when player has 1 card left")
	}
}

func TestReverseTwoPlayers(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindReverse}}
	g.currentTurn = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action":     "play_card",
		"hand_index": 0,
	})
	msgs := g.HandleAction(0, payload)

	if g.currentTurn != 0 {
		t.Errorf("With 2 players reverse should act like skip (same player's turn), got index %d (playerID %d)", g.currentTurn, g.playerIDs[g.currentTurn])
	}

	foundSkipped := false
	for _, m := range msgs {
		if m.Target == BroadcastPID {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "player_skipped" {
				foundSkipped = true
			}
		}
	}
	if !foundSkipped {
		t.Error("Expected player_skipped with 2-player reverse")
	}
}

func TestStatePublic(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	state := g.State(BroadcastPID)
	stateMap, ok := state.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map")
	}
	if stateMap["state"] != StatePlaying {
		t.Errorf("Expected state playing, got %v", stateMap["state"])
	}
	if _, ok := stateMap["turn"]; !ok {
		t.Error("Expected turn in state")
	}
	if _, ok := stateMap["topCard"]; !ok {
		t.Error("Expected topCard in state")
	}
	if _, ok := stateMap["hand"]; ok {
		t.Error("Should not have hand in public state")
	}
}

func TestStatePrivate(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	state := g.State(0)
	stateMap, ok := state.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map")
	}
	hand, ok := stateMap["hand"]
	if !ok {
		t.Fatal("Expected hand in private state")
	}
	cards, ok := hand.([]Card)
	if !ok {
		t.Fatalf("Expected []Card, got %T", hand)
	}
	if len(cards) != 7 {
		t.Errorf("Expected 7 cards in hand, got %d", len(cards))
	}
}

func TestUnknownActionType(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "dance",
	})
	msgs := g.HandleAction(0, payload)
	if len(msgs) == 0 {
		t.Fatal("Expected error message")
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "unknown_game_action" {
		t.Errorf("Expected 'unknown_game_action', got %v", data["error"])
	}
}

func TestInvalidJSONPayload(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})

	msgs := g.HandleAction(0, []byte("not json"))
	if len(msgs) == 0 {
		t.Fatal("Expected error message")
	}
	var data map[string]interface{}
	json.Unmarshal(msgs[0].Data, &data)
	if data["error"] != "invalid_action" {
		t.Errorf("Expected 'invalid_action', got %v", data["error"])
	}
}

func TestGameFinishedNoAction(t *testing.T) {
	g := NewUnoGame(nil)
	g.Start([]PID{0, 1})
	g.state = StateFinished
	g.winner = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "draw_card",
	})
	msgs := g.HandleAction(1, payload)
	if msgs != nil {
		t.Error("Expected no messages when game is finished")
	}
}

func TestPlayAfterDrawConfig(t *testing.T) {
	config := map[string]interface{}{
		"playAfterDraw": false,
	}
	g := NewUnoGame(config)
	g.Start([]PID{0, 1})

	g.discard = []Card{{Color: ColorRed, Kind: KindNumber, Value: 3}}
	g.hands[0] = []Card{{Color: ColorRed, Kind: KindNumber, Value: 5}}
	g.currentTurn = 0

	payload, _ := json.Marshal(map[string]interface{}{
		"action": "draw_card",
	})
	msgs := g.HandleAction(0, payload)

	if g.playAfterDrawIndex != -1 {
		t.Errorf("Expected playAfterDrawIndex -1 when PlayAfterDraw is false, got %d", g.playAfterDrawIndex)
	}

	hasKeepOrPlay := false
	for _, m := range msgs {
		if m.Target == 0 {
			var data map[string]interface{}
			json.Unmarshal(m.Data, &data)
			if data["action"] == "keep_or_play" {
				hasKeepOrPlay = true
			}
		}
	}
	if hasKeepOrPlay {
		t.Error("Should not send keep_or_play when PlayAfterDraw is false")
	}
}

func TestSquashIDs(t *testing.T) {
	tests := []struct {
		input []PID
		want  []PID
	}{
		{[]PID{6, 2, 3, 10}, []PID{2, 0, 1, 3}},
		{[]PID{0, 1, 2}, []PID{0, 1, 2}},
		{[]PID{5, 3, 1}, []PID{2, 1, 0}},
		{[]PID{10}, []PID{0}},
		{nil, nil},
	}
	for _, tt := range tests {
		got := SquashIDs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("SquashIDs(%v) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("SquashIDs(%v) = %v, want %v", tt.input, got, tt.want)
				break
			}
		}
	}
}
