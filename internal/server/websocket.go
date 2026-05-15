package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"PocketDeck/pd-core3/internal/game"
	"PocketDeck/pd-core3/internal/hub"

	"golang.org/x/net/websocket"
)

type ClientState int

const (
	StateStray ClientState = iota
	StateBound
)

type WSC struct {
	ws    *websocket.Conn
	rm    *hub.RoomManager
	room  *hub.Room
	state ClientState

	userID    int
	send      chan []byte
	done      chan struct{}
	closeDone sync.Once
}

func NewWebSocketHandler(rm *hub.RoomManager) http.Handler {
	return websocket.Server{
		Handler: func(ws *websocket.Conn) {
			defer ws.Close()

			log.Println("New connection")

			client := &WSC{
				ws:    ws,
				rm:    rm,
				state: StateStray,
				send:  make(chan []byte, 256),
				done:  make(chan struct{}),
			}

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				client.writePump()
			}()
			go func() {
				defer wg.Done()
				client.readPump()
			}()

			wg.Wait()
			client.shutdown()
		},
	}
}

func (wsc *WSC) markDone() {
	wsc.closeDone.Do(func() { close(wsc.done) })
}

func (wsc *WSC) playerID() game.PID {
	if wsc.room == nil {
		return game.BroadcastPID
	}
	user := wsc.room.GetUser(wsc.userID)
	if user == nil || user.Player == nil {
		return game.BroadcastPID
	}
	return user.Player.ID
}

func (wsc *WSC) leaveRoom() {
	if wsc.room != nil {
		pid := wsc.playerID()
		wsc.room.RemoveUser(wsc.userID)
		if !wsc.room.GameStarted() {
			wsc.room.RemovePlayer(pid)
		}
		wsc.room.BroadcastPlayerUpdate()
	}
}

func (wsc *WSC) shutdown() {
	wsc.leaveRoom()
	wsc.markDone()
	close(wsc.send)
}

func (wsc *WSC) writePump() {
	for {
		select {
		case <-wsc.done:
			return
		case msg, ok := <-wsc.send:
			if !ok {
				return
			}
			if err := websocket.Message.Send(wsc.ws, msg); err != nil {
				log.Println("Write error:", err)
				wsc.markDone()
				return
			}
		}
	}
}

func (wsc *WSC) readPump() {
	defer wsc.markDone()

	for {
		var msg string
		if err := websocket.Message.Receive(wsc.ws, &msg); err != nil {
			log.Println("Read error:", err)
			break
		}
		wsc.handleMessage(msg)
	}
}

func (wsc *WSC) handleMessage(rawMsg string) {
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(rawMsg), &msg); err != nil {
		wsc.sendError("invalid_json")
		return
	}

	action, ok := msg["action"].(string)
	if !ok {
		wsc.sendError("missing_action")
		return
	}

	switch action {
	case "create":
		wsc.handleCreate(msg)
	case "join":
		wsc.handleJoin(msg)
	case "leave":
		wsc.handleLeave()
	case "ready":
		wsc.handleReady(true)
	case "unready":
		wsc.handleReady(false)
	case "status":
		wsc.handleStatus()
	case "game":
		wsc.handleGameAction(msg)
	default:
		wsc.sendError("unknown_action")
	}
}

func (wsc *WSC) handleCreate(msg map[string]interface{}) {
	if wsc.state != StateStray {
		wsc.sendError("not_stray")
		return
	}

	gameTypeStr, ok := msg["game"].(string)
	if !ok {
		wsc.sendError("missing_game")
		return
	}

	gameType := game.GameType(gameTypeStr)
	if !game.IsValidGameType(gameType) {
		wsc.sendError("invalid_game")
		return
	}

	config, _ := msg["config"].(map[string]interface{})
	room := wsc.rm.CreateRoom(gameType, config)
	msg["roomID"] = room.ID
	wsc.handleJoin(msg)
}

func (wsc *WSC) handleJoin(msg map[string]interface{}) {
	if wsc.state != StateStray {
		wsc.sendError("not_stray")
		return
	}

	name, ok := msg["name"].(string)
	if !ok {
		wsc.sendError("missing_name")
		return
	}

	roomID, ok := msg["roomID"].(string)
	if !ok {
		wsc.sendError("missing_room_id")
		return
	}

	room := wsc.rm.GetRoom(roomID)
	if room == nil {
		wsc.sendError("room_not_found")
		return
	}

	wsc.joinRoom(room, name)
}

func (wsc *WSC) joinRoom(room *hub.Room, name string) {
	user := room.AddUser(wsc.send)
	wsc.userID = user.ID

	player := room.GetPlayer(name)
	if player == nil {
		player = room.AddPlayer(name)
	} else if player.IsActive {
		room.RemoveUser(user.ID)
		wsc.sendError("name_taken")
		return
	}

	if !room.ConnectUserToPlayer(user, player.ID) {
		room.RemoveUser(user.ID)
		wsc.sendError("failed_to_connect")
		return
	}

	wsc.room = room
	wsc.state = StateBound

	wsc.sendResponse(map[string]interface{}{
		"action": "joined",
		"roomID": room.ID,
	})

	wsc.sendResponse(map[string]interface{}{
		"action": "navigate",
		"page":   "/lobby",
	})

	room.BroadcastPlayerUpdate()
}

func (wsc *WSC) handleLeave() {
	if wsc.state != StateBound {
		wsc.sendError("not_bound")
		return
	}

	wsc.sendResponse(map[string]interface{}{
		"action": "left",
	})

	wsc.sendResponse(map[string]interface{}{
		"action": "navigate",
		"page":   "/",
	})

	wsc.leaveRoom()
	wsc.room = nil
	wsc.state = StateStray
}

func (wsc *WSC) handleReady(ready bool) {
	if wsc.state != StateBound || wsc.room == nil {
		wsc.sendError("not_bound")
		return
	}

	user := wsc.room.GetUser(wsc.userID)
	if user == nil {
		wsc.sendError("not_bound")
		return
	}

	user.SetReady(ready)

	respAction := "ready"
	if !ready {
		respAction = "unready"
	}

	wsc.sendResponse(map[string]interface{}{
		"action": respAction,
	})

	wsc.room.BroadcastPlayerUpdate()

	if wsc.room.AllReady() {
		wsc.room.Broadcast([]byte(`{"action":"start"}`))
		gamePage := "/games/" + string(wsc.room.GameType())
		navigateMsg, _ := json.Marshal(map[string]interface{}{
			"action": "navigate",
			"page":   gamePage,
		})
		wsc.room.Broadcast(navigateMsg)

		playerIDs := wsc.room.GetPlayerIDs()
		wsc.room.StartGame(playerIDs)
	}
}

func (wsc *WSC) handleStatus() {
	if wsc.state != StateBound || wsc.room == nil {
		wsc.sendError("not_bound")
		return
	}

	user := wsc.room.GetUser(wsc.userID)
	if user == nil {
		wsc.sendError("not_bound")
		return
	}

	players := wsc.room.GetPlayers()
	playerList := make([]map[string]interface{}, 0, len(players))
	for _, p := range players {
		playerList = append(playerList, map[string]interface{}{
			"id":     p.ID,
			"name":   p.Name,
			"points": p.Points,
			"active": p.IsActive,
			"ready":  p.User != nil && p.User.IsReady(),
		})
	}

	gameState := wsc.room.GameState(wsc.playerID())

	resp := map[string]interface{}{
		"action":  "status",
		"roomID":  wsc.room.ID,
		"players": playerList,
		"game":    gameState,
	}

	wsc.sendResponse(resp)
}

func (wsc *WSC) handleGameAction(msg map[string]interface{}) {
	if wsc.state != StateBound || wsc.room == nil {
		wsc.sendError("not_bound")
		return
	}

	gameAction, ok := msg["payload"]
	if !ok {
		wsc.sendError("missing_payload")
		return
	}

	payloadBytes, err := json.Marshal(gameAction)
	if err != nil {
		wsc.sendError("invalid_payload")
		return
	}

	wsc.room.HandleAction(wsc.playerID(), payloadBytes)
}

func (wsc *WSC) sendError(err string) {
	wsc.sendResponse(map[string]interface{}{
		"action": "error",
		"error":  err,
	})
}

func (wsc *WSC) sendResponse(resp map[string]interface{}) {
	respBytes, _ := json.Marshal(resp)
	select {
	case wsc.send <- respBytes:
	default:
		log.Println("Send channel full")
	}
}
