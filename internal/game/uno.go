package game

import (
	"encoding/json"
	"math/rand"
)

type CardColor string

const (
	ColorRed    CardColor = "red"
	ColorBlue   CardColor = "blue"
	ColorGreen  CardColor = "green"
	ColorYellow CardColor = "yellow"
	ColorWild   CardColor = "wild"
)

type CardKind string

const (
	KindNumber    CardKind = "number"
	KindSkip      CardKind = "skip"
	KindReverse   CardKind = "reverse"
	KindDraw2     CardKind = "draw2"
	KindWild      CardKind = "wild"
	KindWildDraw4 CardKind = "wilddraw4"
)

type Card struct {
	Color CardColor `json:"-"`
	Kind  CardKind  `json:"-"`
	Value int       `json:"-"`
}

func (c Card) cardID() int {
	if c.Kind == KindWild || c.Kind == KindWildDraw4 {
		if c.Kind == KindWildDraw4 {
			return 53
		}
		return 52
	}
	colorOrder := map[CardColor]int{"red": 0, "blue": 1, "green": 2, "yellow": 3}
	kindIdx := 0
	switch c.Kind {
	case KindNumber:
		kindIdx = c.Value
	case KindSkip:
		kindIdx = 10
	case KindReverse:
		kindIdx = 11
	case KindDraw2:
		kindIdx = 12
	}
	return colorOrder[c.Color]*13 + kindIdx
}

func (c Card) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{"id": c.cardID()}
	if c.cardID() >= 52 && c.Color != "" && c.Color != ColorWild {
		m["color"] = c.Color
	}
	return json.Marshal(m)
}

type GameState string

const (
	StateWaiting  GameState = "waiting"
	StatePlaying  GameState = "playing"
	StateFinished GameState = "finished"
)

type UnoConfig struct {
	CardsPerPlayer int  `json:"cardsPerPlayer"`
	PointsToWin    int  `json:"pointsToWin"`
	PlayAfterDraw  bool `json:"playAfterDraw"`
	AggregateDraws bool `json:"aggregateDraws"`
	BlackOnBlack   bool `json:"blackOnBlack"`
}

func defaultConfig() UnoConfig {
	return UnoConfig{
		CardsPerPlayer: 7,
		PointsToWin:    500,
		PlayAfterDraw:  true,
		AggregateDraws: true,
		BlackOnBlack:   true,
	}
}

type UnoGame struct {
	state              GameState
	playerIDs          []PID
	hands              map[PID][]Card
	deck               []Card
	discard            []Card
	currentTurn        int
	direction          int
	config             UnoConfig
	drawCounter        int
	playAfterDrawIndex int
	winner             PID
}

func NewUnoGame(config map[string]interface{}) *UnoGame {
	cfg := defaultConfig()
	if v, ok := config["cardsPerPlayer"].(float64); ok {
		cfg.CardsPerPlayer = int(v)
	}
	if v, ok := config["pointsToWin"].(float64); ok {
		cfg.PointsToWin = int(v)
	}
	if v, ok := config["playAfterDraw"].(bool); ok {
		cfg.PlayAfterDraw = v
	}
	if v, ok := config["aggregateDraws"].(bool); ok {
		cfg.AggregateDraws = v
	}
	if v, ok := config["blackOnBlack"].(bool); ok {
		cfg.BlackOnBlack = v
	}
	return &UnoGame{
		state:              StateWaiting,
		config:             cfg,
		direction:          1,
		playAfterDrawIndex: -1,
		winner:             BroadcastPID,
		hands:              make(map[PID][]Card),
	}
}

func (u *UnoGame) Type() GameType {
	return GameUno
}

func (u *UnoGame) Start(playerIDs []PID) []GameMessage {
	u.playerIDs = playerIDs
	u.deck = buildDeck()
	shuffle(u.deck)

	for _, pid := range playerIDs {
		hand := make([]Card, u.config.CardsPerPlayer)
		for i := 0; i < u.config.CardsPerPlayer; i++ {
			hand[i] = *u.drawFromDeck()
		}
		u.hands[pid] = hand
	}

	for {
		u.discard = []Card{*u.drawFromDeck()}
		if u.discard[0].Kind != KindWild && u.discard[0].Kind != KindWildDraw4 {
			break
		}
		u.deck = append(u.deck, u.discard[0])
		shuffle(u.deck)
	}

	u.currentTurn = 0
	u.direction = 1
	u.state = StatePlaying

	return nil
}

func (u *UnoGame) currentPlayerID() PID {
	return u.playerIDs[u.currentTurn]
}

func (u *UnoGame) HandleAction(playerID PID, payload []byte) []GameMessage {
	if u.state == StateFinished {
		return nil
	}

	var action map[string]interface{}
	if err := json.Unmarshal(payload, &action); err != nil {
		return u.errTo(playerID, "invalid_action")
	}

	actionType, _ := action["action"].(string)

	switch actionType {
	case "play_card":
		return u.handlePlay(playerID, action)
	case "draw_card":
		return u.handleDraw(playerID)
	case "keep":
		return u.handleKeep(playerID)
	case "call_uno":
		return u.handleCallUno(playerID)
	case "reorder_hand":
		return u.handleReorder(playerID, action)
	default:
		return u.errTo(playerID, "unknown_game_action")
	}
}

func (u *UnoGame) handlePlay(playerID PID, action map[string]interface{}) []GameMessage {
	if playerID != u.currentPlayerID() {
		return u.errTo(playerID, "not_your_turn")
	}

	idxRaw, ok := action["hand_index"].(float64)
	if !ok {
		return u.errTo(playerID, "missing_hand_index")
	}
	idx := int(idxRaw)

	if u.playAfterDrawIndex >= 0 && idx != u.playAfterDrawIndex {
		return u.errTo(playerID, "must_play_drawn_card")
	}

	hand := u.hands[playerID]
	if idx < 0 || idx >= len(hand) {
		return u.errTo(playerID, "card_not_in_hand")
	}
	card := hand[idx]

	if !u.canPlay(card) {
		return u.errTo(playerID, "cannot_play_card")
	}

	if card.Kind == KindWild || card.Kind == KindWildDraw4 {
		wildColor, hasColor := action["wildColor"].(string)
		if !hasColor || wildColor == "" || wildColor == "wild" {
			return u.errTo(playerID, "must_declare_color")
		}
		card.Color = CardColor(wildColor)
	}

	if u.discard[len(u.discard)-1].Kind == KindWild || u.discard[len(u.discard)-1].Kind == KindWildDraw4 {
		u.discard[len(u.discard)-1].Color = ColorWild
	}

	u.discard = append(u.discard, card)
	u.hands[playerID] = append(hand[:idx], hand[idx+1:]...)
	u.playAfterDrawIndex = -1

	var msgs []GameMessage
	cardJSON, _ := json.Marshal(card)
	playMsg := map[string]interface{}{
		"action":     "card_played",
		"player":     playerID,
		"card":       json.RawMessage(cardJSON),
		"hand_index": idx,
	}
	msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(playMsg)})

	if card.Kind == KindSkip {
		u.advanceTurn()
		skipID := u.currentPlayerID()
		u.advanceTurn()
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "player_skipped",
			"player": skipID,
		})})
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.currentPlayerID(),
		})})
		return msgs
	}

	if card.Kind == KindReverse {
		u.direction *= -1
		if len(u.playerIDs) == 2 {
			skipID := u.playerIDs[u.nextPlayerIndex()]
			msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
				"action": "player_skipped",
				"player": skipID,
			})})
			msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
				"action": "turn",
				"player": u.currentPlayerID(),
			})})
			return msgs
		}
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action":    "direction_reversed",
			"direction": u.direction,
		})})
		return append(msgs, u.nextTurnMsg()...)
	}

	if card.Kind == KindDraw2 {
		u.drawCounter += 2
		nextIdx := u.nextPlayerIndex()
		nextID := u.playerIDs[nextIdx]
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "draw_penalty",
			"player": nextID,
			"count":  2,
		})})
		u.currentTurn = nextIdx
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.currentPlayerID(),
		})})
		return msgs
	}

	if card.Kind == KindWildDraw4 {
		u.drawCounter += 4
		nextIdx := u.nextPlayerIndex()
		nextID := u.playerIDs[nextIdx]
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "draw_penalty",
			"player": nextID,
			"count":  4,
		})})
		u.currentTurn = nextIdx
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.currentPlayerID(),
		})})
		return msgs
	}

	if len(u.hands[playerID]) == 0 {
		u.state = StateFinished
		u.winner = playerID
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "game_over",
			"winner": playerID,
		})})
		return msgs
	}

	if len(u.hands[playerID]) == 1 {
		msgs = append(msgs, GameMessage{Target: BroadcastPID, Data: marshalMsg(map[string]interface{}{
			"action": "uno",
			"player": playerID,
		})})
	}

	return append(msgs, u.nextTurnMsg()...)
}

func (u *UnoGame) handleDraw(playerID PID) []GameMessage {
	if playerID != u.currentPlayerID() {
		return u.errTo(playerID, "not_your_turn")
	}

	var msgs []GameMessage

	if u.drawCounter > 0 {
		var drawn []Card
		for i := 0; i < u.drawCounter; i++ {
			c := u.drawFromDeck()
			if c == nil {
				break
			}
			drawn = append(drawn, *c)
		}
		u.hands[playerID] = append(u.hands[playerID], drawn...)
		u.drawCounter = 0

		drawnJSON, _ := json.Marshal(drawn)
		msgs = append(msgs, GameMessage{Target: playerID, Data: marshalMsg(map[string]interface{}{
			"action": "draw",
			"cards":  json.RawMessage(drawnJSON),
		})})
		msgs = append(msgs, GameMessage{Target: BroadcastExceptCurrentPID, Data: marshalMsg(map[string]interface{}{
			"action": "card_drawn",
			"player": playerID,
			"count":  len(drawn),
		})})

		return append(msgs, u.nextTurnMsg()...)
	}

	c := u.drawFromDeck()
	if c == nil {
		return u.errTo(playerID, "deck_empty")
	}
	u.hands[playerID] = append(u.hands[playerID], *c)

	cardJSON, _ := json.Marshal([]Card{*c})
	msgs = append(msgs, GameMessage{Target: playerID, Data: marshalMsg(map[string]interface{}{
		"action": "draw",
		"cards":  json.RawMessage(cardJSON),
	})})
	msgs = append(msgs, GameMessage{Target: BroadcastExceptCurrentPID, Data: marshalMsg(map[string]interface{}{
		"action": "card_drawn",
		"player": playerID,
		"count":  1,
	})})

	if u.config.PlayAfterDraw && u.canPlay(*c) {
		u.playAfterDrawIndex = len(u.hands[playerID]) - 1
		msgs = append(msgs, GameMessage{Target: playerID, Data: marshalMsg(map[string]interface{}{
			"action":          "keep_or_play",
			"card":            json.RawMessage(cardJSON),
			"played_at_index": u.playAfterDrawIndex,
		})})
		return msgs
	}

	u.playAfterDrawIndex = -1
	return append(msgs, u.nextTurnMsg()...)
}

func (u *UnoGame) handleKeep(playerID PID) []GameMessage {
	if playerID != u.currentPlayerID() {
		return u.errTo(playerID, "not_your_turn")
	}
	if u.playAfterDrawIndex < 0 {
		return u.errTo(playerID, "nothing_to_keep")
	}
	u.playAfterDrawIndex = -1
	return u.nextTurnMsg()
}

func (u *UnoGame) handleCallUno(playerID PID) []GameMessage {
	msgs := u.msg(map[string]interface{}{
		"action": "uno_called",
		"player": playerID,
	})
	return msgs
}

func (u *UnoGame) handleReorder(playerID PID, action map[string]interface{}) []GameMessage {
	f, ok := action["from"].(float64)
	if !ok {
		return u.errTo(playerID, "invalid_from")
	}
	t, ok := action["to"].(float64)
	if !ok {
		return u.errTo(playerID, "invalid_to")
	}

	fromIdx, toIdx := int(f), int(t)
	hand := u.hands[playerID]

	if fromIdx < 0 || fromIdx >= len(hand) {
		return u.errTo(playerID, "invalid_index")
	}
	if toIdx < 0 || toIdx > len(hand) {
		return u.errTo(playerID, "invalid_index")
	}

	card := hand[fromIdx]
	hand = append(hand[:fromIdx], hand[fromIdx+1:]...)
	hand = append(hand[:toIdx], append([]Card{card}, hand[toIdx:]...)...)
	u.hands[playerID] = hand

	return []GameMessage{{
		Target: playerID,
		Data: marshalMsg(map[string]interface{}{
			"action": "hand_reordered",
		}),
	}}
}

func (u *UnoGame) canPlay(card Card) bool {
	if len(u.discard) == 0 {
		return true
	}
	top := u.discard[len(u.discard)-1]

	if u.drawCounter > 0 {
		if !u.config.AggregateDraws {
			return false
		}
		if top.Kind == KindDraw2 {
			return card.Kind == KindDraw2
		}
		if top.Kind == KindWildDraw4 {
			return card.Kind == KindWildDraw4 && u.config.BlackOnBlack
		}
		return false
	}

	if card.Kind == KindWild || card.Kind == KindWildDraw4 {
		canPlay := top.Kind != KindWild && top.Kind != KindWildDraw4
		if u.config.BlackOnBlack {
			canPlay = true
		}
		return canPlay
	}

	if card.Color == top.Color {
		return true
	}

	if card.Kind == top.Kind {
		if card.Kind == KindNumber {
			return card.Value == top.Value
		}
		return true
	}

	return false
}

func (u *UnoGame) nextPlayerIndex() int {
	next := (u.currentTurn + u.direction) % len(u.playerIDs)
	if next < 0 {
		next += len(u.playerIDs)
	}
	return next
}

func (u *UnoGame) advanceTurn() {
	u.currentTurn = u.nextPlayerIndex()
}

func (u *UnoGame) nextTurnMsg() []GameMessage {
	u.advanceTurn()
	return u.msg(map[string]interface{}{
		"action": "turn",
		"player": u.currentPlayerID(),
	})
}

func (u *UnoGame) drawFromDeck() *Card {
	if len(u.deck) == 0 {
		if len(u.discard) <= 1 {
			return nil
		}
		top := u.discard[len(u.discard)-1]
		u.deck = u.discard[:len(u.discard)-1]
		u.discard = []Card{top}
		shuffle(u.deck)
	}
	c := u.deck[len(u.deck)-1]
	u.deck = u.deck[:len(u.deck)-1]
	return &c
}

func (u *UnoGame) broadcastPlayers() []map[string]interface{} {
	var list []map[string]interface{}
	for _, pid := range u.playerIDs {
		entry := map[string]interface{}{
			"id":         pid,
			"card_count": len(u.hands[pid]),
		}
		if pid == u.winner {
			entry["winner"] = true
		}
		list = append(list, entry)
	}
	return list
}

func (u *UnoGame) State(playerID PID) any {
	publicState := map[string]interface{}{
		"state":     u.state,
		"players":   u.broadcastPlayers(),
		"direction": u.direction,
		"drawPile":  len(u.deck),
	}
	if len(u.playerIDs) > 0 {
		publicState["turn"] = u.currentPlayerID()
	}
	if len(u.discard) > 0 {
		publicState["topCard"] = u.discard[len(u.discard)-1]
	}
	if u.winner != BroadcastPID {
		publicState["winner"] = u.winner
	}

	if hand, ok := u.hands[playerID]; ok {
		publicState["hand"] = hand
	}
	if playerID != BroadcastPID {
		publicState["myId"] = playerID
	}
	return publicState
}

func (u *UnoGame) msg(data map[string]interface{}) []GameMessage {
	return []GameMessage{{Target: BroadcastPID, Data: marshalMsg(data)}}
}

func (u *UnoGame) errTo(playerID PID, err string) []GameMessage {
	return []GameMessage{{
		Target: playerID,
		Data: marshalMsg(map[string]interface{}{
			"action": "error",
			"error":  err,
		}),
	}}
}

func buildDeck() []Card {
	var deck []Card
	colors := []CardColor{ColorRed, ColorBlue, ColorGreen, ColorYellow}
	for _, c := range colors {
		deck = append(deck, Card{Color: c, Kind: KindNumber, Value: 0})
		for v := 1; v <= 9; v++ {
			deck = append(deck, Card{Color: c, Kind: KindNumber, Value: v})
			deck = append(deck, Card{Color: c, Kind: KindNumber, Value: v})
		}
		for i := 0; i < 2; i++ {
			deck = append(deck, Card{Color: c, Kind: KindSkip})
			deck = append(deck, Card{Color: c, Kind: KindReverse})
			deck = append(deck, Card{Color: c, Kind: KindDraw2})
		}
	}
	for i := 0; i < 4; i++ {
		deck = append(deck, Card{Color: ColorWild, Kind: KindWild})
		deck = append(deck, Card{Color: ColorWild, Kind: KindWildDraw4})
	}
	return deck
}

func shuffle(deck []Card) {
	rand.Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})
}
