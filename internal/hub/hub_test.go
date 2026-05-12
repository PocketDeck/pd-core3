package hub

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"PocketDeck/pd-core3/internal/game"
)

type MockGame struct {
	actions []string
	mu      sync.Mutex
}

func NewMockGame() *MockGame {
	return &MockGame{}
}

func (m *MockGame) HandleAction(playerID int, payload []byte) []game.GameMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actions = append(m.actions, fmt.Sprintf("%d", playerID))
	return nil
}

func (m *MockGame) Start(playerIDs []int) []game.GameMessage {
	return nil
}

func (m *MockGame) Type() game.GameType {
	return game.GameType("mock")
}

func (m *MockGame) State(playerID int) any {
	return map[string]interface{}{
		"type":   "mock",
		"player": playerID,
	}
}

func (m *MockGame) Actions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.actions))
	copy(result, m.actions)
	return result
}

func makeSendChan() chan []byte {
	return make(chan []byte, 100)
}

func TestRoomCreation(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	if room.ID != "test-room" {
		t.Errorf("Expected room ID 'test-room', got '%s'", room.ID)
	}
}

func TestAddAndGetPlayer(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	player := room.AddPlayer("Alice")
	if player == nil {
		t.Fatal("Expected player to be created")
	}

	if player.Name != "Alice" {
		t.Errorf("Expected player name Alice, got %s", player.Name)
	}

	retrieved := room.GetPlayer("Alice")
	if retrieved != player {
		t.Error("Expected to retrieve the same player")
	}
}

func TestGetNonExistentPlayer(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	player := room.GetPlayer("nobody")
	if player != nil {
		t.Error("Expected nil for non-existent player")
	}
}

func TestGetUser(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	user := room.AddUser(sendChan)
	retrieved := room.GetUser(user.ID)

	if retrieved != user {
		t.Error("Expected to retrieve the same user")
	}
}

func TestGetNonExistentUser(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	user := room.GetUser(999)
	if user != nil {
		t.Error("Expected nil for non-existent user")
	}
}

func TestAddAndRemoveUser(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	user := room.AddUser(sendChan)
	if user == nil {
		t.Fatal("Expected user to be created")
	}

	room.RemoveUser(user.ID)

	if room.GetUser(user.ID) != nil {
		t.Error("Expected user to be removed")
	}
}

func TestRemoveNonExistentUser(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	room.RemoveUser(999)
}

func TestRemoveUserTwice(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	user := room.AddUser(sendChan)
	room.RemoveUser(user.ID)
	room.RemoveUser(user.ID)
}

func TestConnectUserToPlayer(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	player := room.AddPlayer("Alice")
	user := room.AddUser(sendChan)

	success := room.ConnectUserToPlayer(user, "Alice")
	if !success {
		t.Fatal("Expected connection to succeed")
	}

	if user.Player != player {
		t.Error("Expected user to be connected to player")
	}

	if player.User != user {
		t.Error("Expected player to be connected to user")
	}

	if !player.IsActive {
		t.Error("Expected player to be active")
	}
}

func TestConnectUserToNonExistentPlayer(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	user := room.AddUser(sendChan)
	success := room.ConnectUserToPlayer(user, "nobody")
	if success {
		t.Error("Expected connection to fail for non-existent player")
	}
}

func TestConnectUserToPlayerReplacesExistingBinding(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan1 := makeSendChan()
	sendChan2 := makeSendChan()

	player := room.AddPlayer("Alice")

	user1 := room.AddUser(sendChan1)
	room.ConnectUserToPlayer(user1, "Alice")

	user2 := room.AddUser(sendChan2)
	success := room.ConnectUserToPlayer(user2, "Alice")
	if !success {
		t.Fatal("Expected connection to succeed")
	}

	if player.User != user2 {
		t.Error("Expected player to be connected to new user")
	}

	if user1.Player != nil {
		t.Error("Expected old user to be disconnected from player")
	}

	if user2.Player != player {
		t.Error("Expected new user to be connected to player")
	}
}

func TestDisconnectUser(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	player := room.AddPlayer("Alice")
	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")

	room.RemoveUser(user.ID)

	if player.User != nil {
		t.Error("Expected player to be disconnected from user")
	}

	if player.IsActive {
		t.Error("Expected player to be inactive")
	}

	if user.Player != nil {
		t.Error("Expected user to be disconnected from player")
	}
}

func TestPlayerReconnectBySameName(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan1 := makeSendChan()
	sendChan2 := makeSendChan()

	player := room.AddPlayer("Alice")

	user1 := room.AddUser(sendChan1)
	room.ConnectUserToPlayer(user1, "Alice")

	room.RemoveUser(user1.ID)

	user2 := room.AddUser(sendChan2)
	success := room.ConnectUserToPlayer(user2, "Alice")
	if !success {
		t.Fatal("Expected reconnect to succeed")
	}

	if player.User != user2 {
		t.Error("Expected player to be connected to new user")
	}

	if user2.Player != player {
		t.Error("Expected new user to be connected to player")
	}

	if !player.IsActive {
		t.Error("Expected player to be active")
	}
}

func TestMultipleDisconnectReconnect(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	room.AddPlayer("Alice")

	for i := 0; i < 5; i++ {
		user := room.AddUser(makeSendChan())
		room.ConnectUserToPlayer(user, "Alice")

		if !room.GetPlayer("Alice").IsActive {
			t.Fatalf("Expected Alice to be active on iteration %d", i)
		}

		room.RemoveUser(user.ID)

		if room.GetPlayer("Alice").IsActive {
			t.Fatalf("Expected Alice to be inactive after disconnect on iteration %d", i)
		}
	}
}

func TestAllReady(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan1 := makeSendChan()
	sendChan2 := makeSendChan()

	room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	user1 := room.AddUser(sendChan1)
	user2 := room.AddUser(sendChan2)
	room.ConnectUserToPlayer(user1, "Alice")
	room.ConnectUserToPlayer(user2, "Bob")

	if room.AllReady() {
		t.Error("Expected AllReady to be false initially")
	}

	user1.SetReady(true)
	if room.AllReady() {
		t.Error("Expected AllReady to be false when only one user is ready")
	}

	user2.SetReady(true)
	if !room.AllReady() {
		t.Error("Expected AllReady to be true when all users are ready")
	}
}

func TestAllReadySkipsDisconnectedPlayers(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")
	user.SetReady(true)

	if !room.AllReady() {
		t.Error("Expected AllReady to be true when disconnected players are skipped")
	}
}

func TestBroadcast(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan1 := makeSendChan()
	sendChan2 := makeSendChan()

	room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	user1 := room.AddUser(sendChan1)
	user2 := room.AddUser(sendChan2)
	room.ConnectUserToPlayer(user1, "Alice")
	room.ConnectUserToPlayer(user2, "Bob")

	testMsg := []byte(`{"action":"test"}`)
	room.Broadcast(testMsg)

	if string(<-sendChan1) != string(testMsg) {
		t.Error("Expected user1 to receive broadcast")
	}

	if string(<-sendChan2) != string(testMsg) {
		t.Error("Expected user2 to receive broadcast")
	}
}

func TestBroadcastToEmptyRoom(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	room.Broadcast([]byte(`{"action":"test"}`))
}

func TestBroadcastWithFullChannel(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := make(chan []byte, 1)

	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")
	room.AddPlayer("Alice")

	room.Broadcast([]byte(`{"action":"first"}`))
	room.Broadcast([]byte(`{"action":"second"}`))
	room.Broadcast([]byte(`{"action":"third"}`))
}

func TestBroadcastOthers(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan1 := makeSendChan()
	sendChan2 := makeSendChan()

	room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	user1 := room.AddUser(sendChan1)
	user2 := room.AddUser(sendChan2)
	room.ConnectUserToPlayer(user1, "Alice")
	room.ConnectUserToPlayer(user2, "Bob")

	testMsg := []byte(`{"action":"test"}`)
	room.BroadcastOthers(0, testMsg)

	select {
	case msg := <-sendChan1:
		t.Errorf("Expected user1 NOT to receive broadcast, got: %s", msg)
	default:
	}

	if string(<-sendChan2) != string(testMsg) {
		t.Error("Expected user2 to receive broadcast")
	}
}

func TestBroadcastOthersSkipsDisconnectedPlayer(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	alice := room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")

	room.BroadcastOthers(alice.ID, []byte(`{"action":"test"}`))
}

func TestGetPlayers(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	room.AddPlayer("Alice")
	room.AddPlayer("Bob")

	players := room.GetPlayers()
	if len(players) != 2 {
		t.Fatalf("Expected 2 players, got %d", len(players))
	}

	names := make(map[string]bool)
	for _, p := range players {
		names[p.Name] = true
	}

	if !names["Alice"] || !names["Bob"] {
		t.Error("Expected both players to be in the list")
	}
}

func TestGetPlayersReturnsCopy(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	room.AddPlayer("Alice")

	players := room.GetPlayers()
	room.AddPlayer("Bob")

	if len(players) != 1 {
		t.Error("Expected GetPlayers result to be a snapshot")
	}
}

func TestPlayerSend(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	player := room.AddPlayer("Alice")
	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")

	testMsg := []byte(`{"action":"test"}`)
	if !player.Send(testMsg) {
		t.Error("Expected Send to succeed")
	}

	if string(<-sendChan) != string(testMsg) {
		t.Error("Expected message to be sent")
	}
}

func TestPlayerSendWithoutUser(t *testing.T) {
	player := NewPlayer(0, "Alice")
	testMsg := []byte(`{"action":"test"}`)
	if player.Send(testMsg) {
		t.Error("Expected Send to fail when no user is connected")
	}
}

func TestRoomManager(t *testing.T) {
	rm := NewRoomManager()

	room1 := rm.CreateRoom("", nil)
	if room1 == nil {
		t.Fatal("Expected room to be created")
	}

	retrieved := rm.GetRoom(room1.ID)
	if retrieved != room1 {
		t.Error("Expected to retrieve the same room")
	}

	room2 := rm.CreateRoom("", nil)
	if room2.ID == room1.ID {
		t.Error("Expected different room IDs")
	}
}

func TestRoomManagerGetNonExistentRoom(t *testing.T) {
	rm := NewRoomManager()

	room := rm.GetRoom("nope")
	if room != nil {
		t.Error("Expected nil for non-existent room")
	}
}

func TestHandleAction(t *testing.T) {
	mock := NewMockGame()
	room := NewRoom("test-room", "", nil)
	room.SetGame(mock)

	alice := room.AddPlayer("Alice")
	bob := room.AddPlayer("Bob")

	room.HandleAction(alice.ID, []byte(`{"action":"play"}`))
	room.HandleAction(bob.ID, []byte(`{"action":"draw"}`))

	actions := mock.Actions()
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}

	if actions[0] != "0" {
		t.Errorf("Expected first action by player 0, got %s", actions[0])
	}
	if actions[1] != "1" {
		t.Errorf("Expected second action by player 1, got %s", actions[1])
	}
}

func TestHandleActionWithNilGame(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	room.HandleAction(0, []byte(`{"action":"play"}`))
}

func TestGameState(t *testing.T) {
	mock := NewMockGame()
	room := NewRoom("test-room", "", nil)
	room.SetGame(mock)

	alice := room.AddPlayer("Alice")

	state := room.GameState(alice.ID)
	if state == nil {
		t.Fatal("Expected non-nil game state")
	}

	s, ok := state.(map[string]interface{})
	if !ok {
		t.Fatal("Expected map state")
	}

	if s["player"] != alice.ID {
		t.Errorf("Expected player %d in state, got %v", alice.ID, s["player"])
	}
}

func TestGameStateWithNilGame(t *testing.T) {
	room := NewRoom("test-room", "", nil)

	state := room.GameState(0)
	if state != nil {
		t.Error("Expected nil state for nil game")
	}
}

func TestUserTrySend(t *testing.T) {
	sendChan := makeSendChan()
	user := NewUser(1, sendChan)

	msg := []byte(`{"action":"test"}`)
	if !user.TrySend(msg) {
		t.Error("Expected TrySend to succeed")
	}

	if string(<-sendChan) != string(msg) {
		t.Error("Expected to receive the sent message")
	}
}

func TestUserTrySendFullChannel(t *testing.T) {
	sendChan := make(chan []byte, 1)
	user := NewUser(1, sendChan)

	user.TrySend([]byte(`{"action":"first"}`))
	result := user.TrySend([]byte(`{"action":"second"}`))

	if result {
		t.Error("Expected TrySend to fail on full channel")
	}
}

func TestUserReady(t *testing.T) {
	user := NewUser(1, makeSendChan())

	if user.IsReady() {
		t.Error("Expected user to be not ready initially")
	}

	user.SetReady(true)
	if !user.IsReady() {
		t.Error("Expected user to be ready")
	}

	user.SetReady(false)
	if user.IsReady() {
		t.Error("Expected user to be not ready")
	}
}

func TestPlayerJSONSerialization(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	sendChan := makeSendChan()

	room.AddPlayer("Alice")
	user := room.AddUser(sendChan)
	room.ConnectUserToPlayer(user, "Alice")
	user.SetReady(true)

	players := room.GetPlayers()
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

	data, err := json.Marshal(playerList)
	if err != nil {
		t.Fatalf("Expected JSON marshal to succeed, got error: %v", err)
	}

	var unmarshaled []map[string]interface{}
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Expected JSON unmarshal to succeed, got error: %v", err)
	}

	if len(unmarshaled) != 1 {
		t.Fatalf("Expected 1 player in JSON, got %d", len(unmarshaled))
	}

	if unmarshaled[0]["id"] != float64(0) {
		t.Error("Expected player id to be 0")
	}

	if unmarshaled[0]["name"] != "Alice" {
		t.Error("Expected player name to match")
	}

	if unmarshaled[0]["points"] != float64(0) {
		t.Error("Expected points to be 0")
	}

	if unmarshaled[0]["active"] != true {
		t.Error("Expected active to be true")
	}

	if unmarshaled[0]["ready"] != true {
		t.Error("Expected ready to be true")
	}
}

func TestConcurrentAddPlayers(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("Player%d", id)
			room.AddPlayer(name)
		}(i)
	}

	wg.Wait()

	players := room.GetPlayers()
	if len(players) != 10 {
		t.Errorf("Expected 10 players, got %d", len(players))
	}
}

func TestConcurrentBroadcast(t *testing.T) {
	room := NewRoom("test-room", "", nil)
	var users []*User

	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("Player%d", i)
		room.AddPlayer(name)
		user := room.AddUser(makeSendChan())
		room.ConnectUserToPlayer(user, name)
		users = append(users, user)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room.Broadcast([]byte(`{"action":"tick"}`))
		}()
	}

	wg.Wait()
}
