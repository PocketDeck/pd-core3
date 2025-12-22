package main

type User struct {
	ID    int
	Name  string
	Ready bool

	send chan []byte
}

type Room struct {
	ID    string
	Users map[int]*User
	Game  Game

	mu sync.Mutex
}

type RoomManager struct {
	rooms map[string]*Room
	mu sync.Mutex
}

var roomManager = &RoomManager{
	rooms: make(map[string]*Room),
}

func readPump(conn *websocket.Conn, user *User) {
	defer func() {
		conn.Close()
		close(user.send)
		roomManager.RemoveUser(user)
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		handleClientMessage(user, msg)
	}
}

func writePump(conn *websocket.Conn, user *User) {
	for msg := range user.send {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	user := &User{
		ID:  uuid.NewString(),
		Name: "anon",
		send: make(chan []byte, 256)
	}

	go writePump(conn, user)
	readPump(conn, user)
}
