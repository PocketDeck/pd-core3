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
	Color CardColor `json:"color"`
	Kind  CardKind  `json:"kind"`
	Value int       `json:"value,omitempty"`
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
	players            []string
	hands              map[string][]Card
	deck               []Card
	discard            []Card
	currentTurn        int
	direction          int
	config             UnoConfig
	drawCounter        int
	playAfterDrawIndex int
	winner             string
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
	}
}

func (u *UnoGame) Type() GameType {
	return GameUno
}

func (u *UnoGame) Start(players []string) []GameMessage {
	u.players = players
	u.hands = make(map[string][]Card, len(players))
	u.deck = buildDeck()
	shuffle(u.deck)

	for _, name := range players {
		hand := make([]Card, u.config.CardsPerPlayer)
		for i := 0; i < u.config.CardsPerPlayer; i++ {
			hand[i] = *u.drawFromDeck()
		}
		u.hands[name] = hand
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

	return u.broadcastState()
}

func (u *UnoGame) HandleAction(playerName string, payload []byte) []GameMessage {
	if u.state == StateFinished {
		return nil
	}

	var action map[string]interface{}
	if err := json.Unmarshal(payload, &action); err != nil {
		return u.errTo(playerName, "invalid_action")
	}

	actionType, _ := action["action"].(string)

	switch actionType {
	case "play_card":
		return u.handlePlay(playerName, action)
	case "draw_card":
		return u.handleDraw(playerName)
	case "call_uno":
		return u.handleCallUno(playerName)
	case "declare_color":
		return u.handleDeclareColor(playerName, action)
	default:
		return u.errTo(playerName, "unknown_game_action")
	}
}

func (u *UnoGame) handlePlay(playerName string, action map[string]interface{}) []GameMessage {
	if playerName != u.players[u.currentTurn] {
		return u.errTo(playerName, "not_your_turn")
	}

	if u.playAfterDrawIndex >= 0 {
		idx, ok := action["hand_index"].(float64)
		if !ok || int(idx) != u.playAfterDrawIndex {
			return u.errTo(playerName, "must_play_drawn_card")
		}
	}

	var card Card
	cardRaw, ok := action["card"].(map[string]interface{})
	if !ok {
		return u.errTo(playerName, "missing_card")
	}
	card.Color = CardColor(cardRaw["color"].(string))
	card.Kind = CardKind(cardRaw["kind"].(string))
	if v, ok := cardRaw["value"].(float64); ok {
		card.Value = int(v)
	}

	hand := u.hands[playerName]
	idx := -1
	for i, c := range hand {
		if cardsEqual(c, card) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return u.errTo(playerName, "card_not_in_hand")
	}

	if !u.canPlay(card) {
		return u.errTo(playerName, "cannot_play_card")
	}

	if card.Kind == KindWild || card.Kind == KindWildDraw4 {
		wildColor, hasColor := action["wildColor"].(string)
		if !hasColor || wildColor == "" || wildColor == "wild" {
			return u.errTo(playerName, "must_declare_color")
		}
		card.Color = CardColor(wildColor)
	}

	if u.discard[len(u.discard)-1].Kind == KindWild || u.discard[len(u.discard)-1].Kind == KindWildDraw4 {
		u.discard[len(u.discard)-1].Color = ColorWild
	}

	u.discard = append(u.discard, card)
	u.hands[playerName] = append(hand[:idx], hand[idx+1:]...)
	u.playAfterDrawIndex = -1

	var msgs []GameMessage
	cardJSON, _ := json.Marshal(card)
	playMsg := map[string]interface{}{
		"action":     "card_played",
		"player":     playerName,
		"card":       json.RawMessage(cardJSON),
		"hand_index": idx,
	}
	msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(playMsg)})

	if card.Kind == KindSkip {
		u.advanceTurn()
		skipPlayer := u.players[u.currentTurn]
		u.advanceTurn()
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "player_skipped",
			"player": skipPlayer,
		})})
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.players[u.currentTurn],
		})})
		return msgs
	}

	if card.Kind == KindReverse {
		u.direction *= -1
		if len(u.players) == 2 {
			skipPlayer := u.players[u.nextPlayerIndex()]
			msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
				"action": "player_skipped",
				"player": skipPlayer,
			})})
			msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
				"action": "turn",
				"player": u.players[u.currentTurn],
			})})
			return msgs
		}
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action":    "direction_reversed",
			"direction": u.direction,
		})})
		return append(msgs, u.nextTurnMsg()...)
	}

	if card.Kind == KindDraw2 {
		u.drawCounter += 2
		nextIdx := u.nextPlayerIndex()
		nextPlayer := u.players[nextIdx]
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "draw_penalty",
			"player": nextPlayer,
			"count":  2,
		})})
		u.currentTurn = nextIdx
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.players[u.currentTurn],
		})})
		return msgs
	}

	if card.Kind == KindWildDraw4 {
		u.drawCounter += 4
		nextIdx := u.nextPlayerIndex()
		nextPlayer := u.players[nextIdx]
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "draw_penalty",
			"player": nextPlayer,
			"count":  4,
		})})
		u.currentTurn = nextIdx
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "turn",
			"player": u.players[u.currentTurn],
		})})
		return msgs
	}

	if len(u.hands[playerName]) == 0 {
		u.state = StateFinished
		u.winner = playerName
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "game_over",
			"winner": playerName,
		})})
		return msgs
	}

	if len(u.hands[playerName]) == 1 {
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "uno",
			"player": playerName,
		})})
	}

	return append(msgs, u.nextTurnMsg()...)
}

func (u *UnoGame) handleDraw(playerName string) []GameMessage {
	if playerName != u.players[u.currentTurn] {
		return u.errTo(playerName, "not_your_turn")
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
		u.hands[playerName] = append(u.hands[playerName], drawn...)
		u.drawCounter = 0

		drawnJSON, _ := json.Marshal(drawn)
		msgs = append(msgs, GameMessage{Target: playerName, Data: marshalMsg(map[string]interface{}{
			"action": "draw",
			"cards":  json.RawMessage(drawnJSON),
		})})
		msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
			"action": "card_drawn",
			"player": playerName,
			"count":  len(drawn),
		})})

		return append(msgs, u.nextTurnMsg()...)
	}

	c := u.drawFromDeck()
	if c == nil {
		return u.errTo(playerName, "deck_empty")
	}
	u.hands[playerName] = append(u.hands[playerName], *c)

	cardJSON, _ := json.Marshal([]Card{*c})
	msgs = append(msgs, GameMessage{Target: playerName, Data: marshalMsg(map[string]interface{}{
		"action": "draw",
		"cards":  json.RawMessage(cardJSON),
	})})
	msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(map[string]interface{}{
		"action": "card_drawn",
		"player": playerName,
		"count":  1,
	})})

	if u.config.PlayAfterDraw && u.canPlay(*c) {
		u.playAfterDrawIndex = len(u.hands[playerName]) - 1
		msgs = append(msgs, GameMessage{Target: playerName, Data: marshalMsg(map[string]interface{}{
			"action":         "keep_or_play",
			"card":           json.RawMessage(cardJSON),
			"played_at_index": u.playAfterDrawIndex,
		})})
		return msgs
	}

	u.playAfterDrawIndex = -1
	return append(msgs, u.nextTurnMsg()...)
}

func (u *UnoGame) handleCallUno(playerName string) []GameMessage {
	msgs := u.msg(map[string]interface{}{
		"action": "uno_called",
		"player": playerName,
	})
	return msgs
}

func (u *UnoGame) handleDeclareColor(playerName string, action map[string]interface{}) []GameMessage {
	color, ok := action["color"].(string)
	if !ok || color == "" || color == "wild" {
		return u.errTo(playerName, "invalid_color")
	}
	if playerName != u.players[u.currentTurn] {
		return u.errTo(playerName, "not_your_turn")
	}
	hand := u.hands[playerName]
	idx, ok := action["hand_index"].(float64)
	if !ok || int(idx) >= len(hand) {
		return u.errTo(playerName, "invalid_hand_index")
	}
	card := hand[int(idx)]
	if card.Kind != KindWild && card.Kind != KindWildDraw4 {
		return u.errTo(playerName, "not_a_wild_card")
	}
	u.hands[playerName][int(idx)] = Card{
		Color: CardColor(color),
		Kind:  card.Kind,
		Value: 0,
	}
	msgs := u.msg(map[string]interface{}{
		"action": "color_declared",
		"player": playerName,
		"color":  color,
	})
	return msgs
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
		return true
	}

	return false
}

func (u *UnoGame) nextPlayerIndex() int {
	next := (u.currentTurn + u.direction) % len(u.players)
	if next < 0 {
		next += len(u.players)
	}
	return next
}

func (u *UnoGame) advanceTurn() {
	u.currentTurn = u.nextPlayerIndex()
}

func (u *UnoGame) nextTurnMsg() []GameMessage {
	u.advanceTurn()
	player := u.players[u.currentTurn]
	return u.msg(map[string]interface{}{
		"action": "turn",
		"player": player,
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

func (u *UnoGame) broadcastState() []GameMessage {
	turnPlayer := ""
	if len(u.players) > 0 {
		turnPlayer = u.players[u.currentTurn]
	}

	publicState := map[string]interface{}{
		"action":    "game_state",
		"state":     u.state,
		"players":   u.broadcastPlayers(),
		"turn":      turnPlayer,
		"direction": u.direction,
		"drawPile":  len(u.deck),
	}
	if len(u.discard) > 0 {
		publicState["topCard"] = u.discard[len(u.discard)-1]
	}
	if u.winner != "" {
		publicState["winner"] = u.winner
	}

	var msgs []GameMessage
	msgs = append(msgs, GameMessage{Target: "", Data: marshalMsg(publicState)})

	for _, name := range u.players {
		privateState := map[string]interface{}{
			"hand": u.hands[name],
		}
		msgs = append(msgs, GameMessage{Target: name, Data: marshalMsg(privateState)})
	}
	return msgs
}

func (u *UnoGame) broadcastPlayers() []map[string]interface{} {
	var list []map[string]interface{}
	for _, name := range u.players {
		entry := map[string]interface{}{
			"name":       name,
			"card_count": len(u.hands[name]),
		}
		if name == u.winner {
			entry["winner"] = true
		}
		list = append(list, entry)
	}
	return list
}

func (u *UnoGame) State(playerName string) any {
	publicState := map[string]interface{}{
		"state":     u.state,
		"players":   u.broadcastPlayers(),
		"direction": u.direction,
		"drawPile":  len(u.deck),
	}
	if len(u.players) > 0 {
		publicState["turn"] = u.players[u.currentTurn]
	}
	if len(u.discard) > 0 {
		publicState["topCard"] = u.discard[len(u.discard)-1]
	}
	if u.winner != "" {
		publicState["winner"] = u.winner
	}

	if playerName != "" {
		if hand, ok := u.hands[playerName]; ok {
			publicState["hand"] = hand
		}
	}
	return publicState
}

func (u *UnoGame) msg(data map[string]interface{}) []GameMessage {
	return []GameMessage{{Target: "", Data: marshalMsg(data)}}
}

func (u *UnoGame) errTo(playerName string, err string) []GameMessage {
	return []GameMessage{{
		Target: playerName,
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

func cardsEqual(a, b Card) bool {
	return a.Color == b.Color && a.Kind == b.Kind && a.Value == b.Value
}
