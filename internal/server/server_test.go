package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"PocketDeck/pd-core3/internal/hub"

	"golang.org/x/net/websocket"
)

func dialServer(t *testing.T, mux *http.ServeMux) (*websocket.Conn, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(mux)
	t.Cleanup(func() { server.Close() })

	wsURL := "ws" + server.URL[4:] + "/"
	ws, err := websocket.Dial(wsURL, "", "http://localhost/")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	t.Cleanup(func() { ws.Close() })

	return ws, server
}

func sendMsg(t *testing.T, ws *websocket.Conn, msg map[string]interface{}) {
	t.Helper()

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	if err := websocket.Message.Send(ws, string(data)); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}
}

func recvMsg(t *testing.T, ws *websocket.Conn) map[string]interface{} {
	t.Helper()

	var respStr string
	if err := websocket.Message.Receive(ws, &respStr); err != nil {
		t.Fatalf("Failed to receive: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(respStr), &resp); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	return resp
}

func recvMsgTimeout(t *testing.T, ws *websocket.Conn, timeout time.Duration) (map[string]interface{}, bool) {
	t.Helper()

	type result struct {
		msg map[string]interface{}
		err error
	}

	ch := make(chan result, 1)
	go func() {
		var respStr string
		err := websocket.Message.Receive(ws, &respStr)
		if err != nil {
			ch <- result{nil, err}
			return
		}
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(respStr), &resp); err != nil {
			ch <- result{nil, err}
			return
		}
		ch <- result{resp, nil}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			return nil, false
		}
		return r.msg, true
	case <-time.After(timeout):
		return nil, false
	}
}

func makeMux(rm *hub.RoomManager) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", NewWebSocketHandler(rm))
	return mux
}

func TestIntegrationCreateRoom(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})

	resp := recvMsg(t, ws)
	if resp["action"] != "joined" {
		t.Errorf("Expected 'joined', got %v", resp["action"])
	}

	roomID, ok := resp["roomID"].(string)
	if !ok || roomID == "" {
		t.Error("Expected non-empty roomID")
	}

	if rm.GetRoom(roomID) == nil {
		t.Error("Expected room to exist in manager")
	}
}

func TestIntegrationCreateRoomInvalidGame(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "pokemon",
	})

	resp := recvMsg(t, ws)
	assertError(t, resp, "invalid_game")
}

func TestIntegrationCreateRoomMissingGame(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
	})

	resp := recvMsg(t, ws)
	assertError(t, resp, "missing_game")
}

func TestIntegrationJoinRoom(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)

	// clear the navigate and players broadcast from create
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	resp := recvMsg(t, ws2)
	assertAction(t, resp, "joined")

	if resp["roomID"] != roomID {
		t.Errorf("Expected roomID %s, got %v", roomID, resp["roomID"])
	}
}

func TestIntegrationJoinNonExistentRoom(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "join",
		"name":   "Alice",
		"roomID": "NONEXIST",
	})

	resp := recvMsg(t, ws)
	assertError(t, resp, "room_not_found")
}

func TestIntegrationJoinMissingFields(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "join",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "missing_name")

	sendMsg(t, ws, map[string]interface{}{
		"action": "join",
		"name":   "Alice",
	})
	resp = recvMsg(t, ws)
	assertError(t, resp, "missing_room_id")
}

func TestIntegrationLeaveRoom(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	sendMsg(t, ws, map[string]interface{}{
		"action": "leave",
	})
	resp := recvMsg(t, ws)
	assertAction(t, resp, "left")
	recvMsg(t, ws) // navigate
}

func TestIntegrationLeaveWhenStray(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "leave",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "not_bound")
}

func TestIntegrationCreateAfterLeave(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	sendMsg(t, ws, map[string]interface{}{
		"action": "leave",
	})
	recvMsg(t, ws) // left
	recvMsg(t, ws) // navigate

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice2",
		"game":   "uno",
	})
	resp := recvMsg(t, ws)
	assertAction(t, resp, "joined")
}

func TestIntegrationReconnect(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	// First connection disconnects (close)
	ws1.Close()
	time.Sleep(50 * time.Millisecond) // allow async cleanup

	// Reconnect with same name
	ws2, srv := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Alice",
		"roomID": roomID,
	})
	resp := recvMsg(t, ws2)
	assertAction(t, resp, "joined")
	_ = srv
	ws2.Close()
}

func TestIntegrationNameTaken(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	// Second connection tries same name while Alice is active
	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Alice",
		"roomID": roomID,
	})
	resp := recvMsg(t, ws2)
	assertError(t, resp, "name_taken")
}

func TestIntegrationReadyAndStart(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players broadcast

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2)           // joined
	recvMsg(t, ws2)           // navigate
	recvMsg(t, ws2)           // players broadcast (from Bob joining)
	recvMsg(t, ws1)           // players broadcast (from Bob joining)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "ready",
	})
	recvMsg(t, ws1) // ready ack
	recvMsg(t, ws2) // players broadcast (Alice ready)
	recvMsg(t, ws1) // players broadcast (Alice ready)

	sendMsg(t, ws2, map[string]interface{}{
		"action": "ready",
	})
	recvMsg(t, ws2) // ready ack
	// After the ready ack, there's a players broadcast, then start, then navigate
	recvMsg(t, ws1) // players broadcast (Bob ready)
	recvMsg(t, ws2) // players broadcast (Bob ready)

	start1 := recvMsg(t, ws1)
	start2 := recvMsg(t, ws2)
	assertAction(t, start1, "start")
	assertAction(t, start2, "start")
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws2) // navigate
}

func TestIntegrationUnready(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)
	ws2, _ := dialServer(t, mux)

	// Alice creates room
	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	// Bob joins so that AllReady doesn't trigger with just Alice
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2) // joined
	recvMsg(t, ws2) // navigate
	recvMsg(t, ws2) // players
	recvMsg(t, ws1) // players

	// Alice readies
	sendMsg(t, ws1, map[string]interface{}{
		"action": "ready",
	})
	recvMsg(t, ws1) // ready ack
	recvMsg(t, ws2) // players (Alice ready)
	recvMsg(t, ws1) // players (Alice ready)

	// Alice unreadies — no start should happen since Bob isn't ready
	sendMsg(t, ws1, map[string]interface{}{
		"action": "unready",
	})
	recvMsg(t, ws2) // players (Alice unready)
	resp := recvMsg(t, ws1)
	assertAction(t, resp, "unready")
}

func TestIntegrationReadyNotBound(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "ready",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "not_bound")
}

func TestIntegrationStatus(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws1) // joined
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	sendMsg(t, ws1, map[string]interface{}{
		"action": "status",
	})
	resp := recvMsg(t, ws1)
	assertAction(t, resp, "status")

	if resp["roomID"] == "" {
		t.Error("Expected non-empty roomID in status")
	}

	players, ok := resp["players"].([]interface{})
	if !ok || len(players) != 1 {
		t.Fatalf("Expected 1 player in status, got %v", players)
	}

	player := players[0].(map[string]interface{})
	if player["name"] != "Alice" {
		t.Errorf("Expected player name Alice, got %v", player["name"])
	}
}

func TestIntegrationStatusNotBound(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "status",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "not_bound")
}

func TestIntegrationGameAction(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	sendMsg(t, ws, map[string]interface{}{
		"action":   "game",
		"payload":  map[string]interface{}{"action": "play", "from": 0},
	})
}

func TestIntegrationGameActionMissingPayload(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	sendMsg(t, ws, map[string]interface{}{
		"action": "game",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "missing_payload")
}

func TestIntegrationInvalidJSON(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	if err := websocket.Message.Send(ws, "not json"); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	resp := recvMsg(t, ws)
	assertError(t, resp, "invalid_json")
}

func TestIntegrationMissingAction(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"name": "Alice",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "missing_action")
}

func TestIntegrationUnknownAction(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "dance",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "unknown_action")
}

func TestIntegrationNotStrayErrors(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	// Should error since we're bound
	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "not_stray")

	sendMsg(t, ws, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": "SOMEROOM",
	})
	resp = recvMsg(t, ws)
	assertError(t, resp, "not_stray")
}

func TestIntegrationPlayersBroadcastOnJoin(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	joined := recvMsg(t, ws1)
	roomID := joined["roomID"].(string)

	// Navigate
	navigateMsg := recvMsg(t, ws1)
	assertAction(t, navigateMsg, "navigate")

	// First broadcast: Alice joins
	playersMsg := recvMsg(t, ws1)
	assertAction(t, playersMsg, "players")

	players, _ := playersMsg["players"].([]interface{})
	if len(players) != 1 {
		t.Fatalf("Expected 1 player, got %d", len(players))
	}

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2) // joined
	recvMsg(t, ws2) // navigate

	// ws2 gets players broadcast
	playersMsg2 := recvMsg(t, ws2)
	assertAction(t, playersMsg2, "players")

	players2, _ := playersMsg2["players"].([]interface{})
	if len(players2) != 2 {
		t.Fatalf("Expected 2 players, got %d", len(players2))
	}

	// ws1 also gets players broadcast
	playersMsg1b := recvMsg(t, ws1)
	assertAction(t, playersMsg1b, "players")
}

func TestIntegrationConcurrentCreateRoom(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)

	var wg sync.WaitGroup
	roomIDs := make(chan string, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ws, _ := dialServer(t, mux)
			sendMsg(t, ws, map[string]interface{}{
				"action": "create",
				"name":   fmt.Sprintf("Player%d", id),
				"game":   "uno",
			})
			resp := recvMsg(t, ws)
			if resp["action"] != "joined" {
				t.Errorf("Expected 'joined', got %v", resp["action"])
			}
			roomID, ok := resp["roomID"].(string)
			if ok {
				roomIDs <- roomID
			}
		}(i)
	}

	wg.Wait()
	close(roomIDs)

	ids := make(map[string]bool)
	for id := range roomIDs {
		if ids[id] {
			t.Error("Duplicate room ID")
		}
		ids[id] = true
	}

	if len(ids) != 5 {
		t.Errorf("Expected 5 unique room IDs, got %d", len(ids))
	}
}

func assertAction(t *testing.T, resp map[string]interface{}, expected string) {
	t.Helper()

	if resp["action"] != expected {
		t.Errorf("Expected action '%s', got '%v'", expected, resp["action"])
	}
}

func assertError(t *testing.T, resp map[string]interface{}, expectedErr string) {
	t.Helper()

	if resp["action"] != "error" {
		t.Errorf("Expected 'error' action, got '%v'", resp["action"])
	}

	if resp["error"] != expectedErr {
		t.Errorf("Expected error '%s', got '%v'", expectedErr, resp["error"])
	}
}

func TestIntegrationJoinThenStatusShowsTwoPlayers(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	joined := recvMsg(t, ws1)
	roomID := joined["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players (Alice)

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2) // joined

	// ws1 may get a players broadcast from Bob's join before our status
	// Check status from ws1 shows 2 players
	sendMsg(t, ws1, map[string]interface{}{
		"action": "status",
	})
	for i := 0; i < 5; i++ {
		msg := recvMsg(t, ws1)
		if msg["action"] == "status" {
			players, _ := msg["players"].([]interface{})
			if len(players) == 2 {
				return
			}
			t.Fatalf("Expected 2 players in status, got %d", len(players))
		}
	}
	t.Fatal("Did not receive status response")
}

func TestIntegrationJoinAndReadyMultiple(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	createResp := recvMsg(t, ws1)
	roomID := createResp["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2)           // joined
	recvMsg(t, ws2)           // navigate
	recvMsg(t, ws2)           // players
	recvMsg(t, ws1)           // players

	// Alice readies
	sendMsg(t, ws1, map[string]interface{}{"action": "ready"})
	recvMsg(t, ws1) // ack
	recvMsg(t, ws2) // players
	recvMsg(t, ws1) // players

	// Bob readies — triggers start
	sendMsg(t, ws2, map[string]interface{}{"action": "ready"})
	recvMsg(t, ws2)  // ack
	recvMsg(t, ws1)  // players (Bob ready)
	recvMsg(t, ws2)  // players (Bob ready)

	// Both should get start
	start1, ok1 := recvMsgTimeout(t, ws1, time.Second)
	start2, ok2 := recvMsgTimeout(t, ws2, time.Second)

	if !ok1 || !ok2 {
		t.Fatal("Expected both to receive start broadcast")
	}
	if start1["action"] != "start" || start2["action"] != "start" {
		t.Error("Expected 'start' action for both")
	}
	// Both should get navigate after start
	nav1, ok1 := recvMsgTimeout(t, ws1, time.Second)
	nav2, ok2 := recvMsgTimeout(t, ws2, time.Second)
	if !ok1 || !ok2 {
		t.Fatal("Expected both to receive navigate broadcast")
	}
	if nav1["action"] != "navigate" || nav2["action"] != "navigate" {
		t.Error("Expected 'navigate' action for both")
	}
}

func TestIntegrationUnoCreateWithConfig(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
		"config": map[string]interface{}{
			"cardsPerPlayer": float64(5),
			"playAfterDraw":  false,
		},
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players
}

func TestIntegrationUnoCreateWithInvalidConfig(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
		"config": "not_a_map",
	})
	recvMsg(t, ws) // joined (config is silently ignored on bad type)
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players
}

func TestIntegrationUnoPlayCard(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws1, _ := dialServer(t, mux)

	sendMsg(t, ws1, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	joined := recvMsg(t, ws1)
	roomID := joined["roomID"].(string)
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws1) // players

	ws2, _ := dialServer(t, mux)
	sendMsg(t, ws2, map[string]interface{}{
		"action": "join",
		"name":   "Bob",
		"roomID": roomID,
	})
	recvMsg(t, ws2) // joined
	recvMsg(t, ws2) // navigate
	recvMsg(t, ws2) // players
	recvMsg(t, ws1) // players

	// Both ready
	sendMsg(t, ws1, map[string]interface{}{"action": "ready"})
	recvMsg(t, ws1) // ready
	recvMsg(t, ws2) // players
	recvMsg(t, ws1) // players

	sendMsg(t, ws2, map[string]interface{}{"action": "ready"})
	recvMsg(t, ws2) // ready
	recvMsg(t, ws1) // players
	recvMsg(t, ws2) // players
	recvMsg(t, ws1) // start
	recvMsg(t, ws2) // start
	recvMsg(t, ws1) // navigate
	recvMsg(t, ws2) // navigate

	// After start: game broadcasts game_state (public) + hand (private) per player
	gameState := recvMsg(t, ws1)
	if gameState["action"] != "game_state" {
		t.Errorf("Expected game_state, got %v", gameState["action"])
	}

	hand1 := recvMsg(t, ws1)
	if _, ok := hand1["hand"]; !ok {
		t.Errorf("Expected private hand for Alice, got %v", hand1)
	}

	gameState2 := recvMsg(t, ws2)
	if gameState2["action"] != "game_state" {
		t.Errorf("Expected game_state for Bob, got %v", gameState2["action"])
	}

	hand2 := recvMsg(t, ws2)
	if _, ok := hand2["hand"]; !ok {
		t.Errorf("Expected private hand for Bob, got %v", hand2)
	}

	// Send a game action
	sendMsg(t, ws1, map[string]interface{}{
		"action": "game",
		"payload": map[string]interface{}{
			"action": "draw_card",
		},
	})
	drawResp := recvMsg(t, ws1)
	if drawResp["action"] != "draw" {
		t.Errorf("Expected draw action, got %v", drawResp["action"])
	}
}

func TestIntegrationUnoGameActionNotBound(t *testing.T) {
	mux := makeMux(hub.NewRoomManager())
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "game",
		"payload": map[string]interface{}{
			"action": "draw_card",
		},
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "not_bound")
}

func TestIntegrationUnoGameActionMissingPayload(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws, _ := dialServer(t, mux)

	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	recvMsg(t, ws) // joined
	recvMsg(t, ws) // navigate
	recvMsg(t, ws) // players

	sendMsg(t, ws, map[string]interface{}{
		"action": "game",
	})
	resp := recvMsg(t, ws)
	assertError(t, resp, "missing_payload")
}

func TestIntegrationCreateAfterJoin(t *testing.T) {
	rm := hub.NewRoomManager()
	mux := makeMux(rm)
	ws, _ := dialServer(t, mux)

	// Join non-existent room (gets error)
	sendMsg(t, ws, map[string]interface{}{
		"action": "join",
		"name":   "Alice",
		"roomID": "NOPE",
	})
	recvMsg(t, ws) // error

	// Now create — should work since we're still stray
	sendMsg(t, ws, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"game":   "uno",
	})
	resp := recvMsg(t, ws)
	assertAction(t, resp, "joined")
}


