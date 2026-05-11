package hub

import (
	"crypto/rand"
	"sync"

	"PocketDeck/pd-core3/internal/game"
)

type RoomManager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]*Room),
	}
}

func (rm *RoomManager) GetRoom(roomID string) *Room {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.rooms[roomID]
}

const IDLength = 6

func (rm *RoomManager) CreateRoom(gameType game.GameType, config map[string]interface{}) *Room {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// generate new ID
	//  - use crypto/rand for random ID
	//  - loop while ID exists
	var ID string
	for {
		b := make([]byte, IDLength)
		rand.Read(b)

		// convert number to string
		for i := range IDLength {
			b[i] = b[i] % 36
			if b[i] < 10 {
				b[i] += '0'
			} else {
				b[i] += 'a' - 10
			}
		}
		ID = string(b)

		// check if ID exists
		if _, ok := rm.rooms[ID]; !ok {
			break
		}
	}

	room := NewRoom(ID, gameType, config)
	rm.rooms[ID] = room
	return room
}

