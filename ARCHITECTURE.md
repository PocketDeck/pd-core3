# PocketDeck Server — Architecture & Implementation Plan

## 1. Project Vision

PocketDeck is a **WebSocket-based multiplayer card game server**. Clients connect via WebSocket, create or join game rooms, and play card games (Uno, Skip-Bo, etc.). The server manages:

- **WebSocket connections** — real-time bidirectional communication
- **Room management** — create/join/leave game rooms
- **Player lifecycle** — persistent player entities with reconnection support
- **Game engines** — pluggable card game implementations
- **Message broadcasting** — relay game state and events to players

---

## 2. High-Level Architecture

```
┌─────────────┐     WebSocket      ┌───────────────────┐
│   Client 1   │ ◄──────────────► │                   │
└─────────────┘                    │   WebSocket       │
                                   │   Handler         │
┌─────────────┐                    │   (server/)       │
│   Client 2   │ ◄──────────────► │                   │
└─────────────┘                    └────────┬──────────┘
                                            │
                                   ┌────────▼──────────┐
                                   │   Room Manager     │
                                   │   (hub/)           │
                                   │                    │
                                   │   ┌──────────┐    │
                                   │   │  Room A   │    │
                                   │   │ Players[] │    │
                                   │   │ Users[]   │    │
                                   │   │ Game      │    │
                                   │   └──────────┘    │
                                   │   ┌──────────┐    │
                                   │   │  Room B   │    │
                                   │   │ ...      │    │
                                   │   └──────────┘    │
                                   └────────┬──────────┘
                                            │
                                   ┌────────▼──────────┐
                                   │  Game Engines      │
                                   │  (game/)           │
                                   │  ┌─────┐ ┌──────┐ │
                                   │  │ Uno │ │SkipBo│ │
                                   │  └─────┘ └──────┘ │
                                   └───────────────────┘
```

### Message Flow

```
Client ──{"action":"create",...}──► WSC.readPump()
                                          │
                                    handleCreate()
                                          │
                                    RoomManager.CreateRoom(type, config)
                                    Room.AddPlayer()
                                    Room.AddUser()
                                    Room.ConnectUserToPlayer()
                                          │
                                    WSC.send ──{"action":"joined",...}──► Client
                                    Room.BroadcastPlayerUpdate()
                                          │
                                    ──► All Users in Room
```

---

## 3. Core Domain Model

### 3.1 User (internal/hub/user.go)

Represents a **live WebSocket connection session** — ephemeral, created per connection.

```go
type User struct {
    ID     int          // Auto-incrementing per-room
    Player *Player      // Back-reference to associated Player (nil if stray)
    ready  atomic.Bool  // Ready state for game start
    Send   chan []byte  // Buffered outgoing message channel (cap 256)
}
```

**Key behaviors:**
- Created when a WebSocket connects and joins a room
- Destroyed when the WebSocket disconnects or leaves
- `TrySend()` — non-blocking send (drops if channel full)
- The `Send` channel is drained by the WebSocket's `writePump` goroutine

### 3.2 Player (internal/hub/player.go)

Represents a **persistent in-game entity** — survives disconnects.

```go
type Player struct {
    ID       int        // Assigned sequentially per-room (0, 1, 2, ...)
    Name     string     // Unique per room, chosen by client
    Points   int        // Running score
    IsActive bool       // Currently connected to a User
    Room     *Room      // Back-reference to parent Room
    User     *User      // Back-reference to connected User (nil if disconnected)
}
```

**Key behaviors:**
- Created on first join, persists across reconnections
- Reconnecting: client sends same `name` → server finds inactive Player → connects new User
- `Send()` — delegates to User's `TrySend()`, returns false if no User connected
- Player ID is used by the game engine (instead of name) for all game events

### 3.3 Room (internal/hub/room.go)

A **game room** — the central coordination point.

```go
type Room struct {
	ID      string              // 6-char alphanumeric, crypto-random
	players map[string]*Player  // Persistent players (keyed by name)
	users   map[int]*User       // Active connections (keyed by user ID)
	game    game.Game           // Pluggable game engine (nil until start)
	gameType    game.GameType   // Game type, set at creation
	gameConfig  map[string]interface{} // Game config, set at creation
	mu      sync.RWMutex        // Thread safety
	nextPlayerID int            // Auto-incrementing player ID counter (0, 1, 2, ...)
	userID  int                 // Auto-incrementing user ID counter
}
```

**Key responsibilities:**
- Add/remove players and users
- Connect users to players (binds session to entity)
- Broadcast messages to all users (or all except one)
- Store game type + config at creation (game is NOT instantiated yet)
- Create game engine on first `StartGame()` call (when all players ready)
- Forward game actions to the game engine
- Track ready state → emit `start` when all ready

### 3.4 RoomManager (internal/hub/room_manager.go)

Manages all rooms globally.

```go
type RoomManager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}
```

**Key responsibilities:**
- `CreateRoom(gameType, config) *Room` — generates unique 6-char ID, stores room with game type + config
- `GetRoom(roomID string) *Room` — lookup by ID

The game engine is NOT created here. It is created lazily when `Room.StartGame()` is called, so that player list and config are finalized before the game deals cards.

### 3.5 Game Interface (internal/game/game.go)

```go
type GameMessage struct {
    Target int              // -1 = broadcast, otherwise player ID
    Data   json.RawMessage
}

type Game interface {
    HandleAction(playerID int, payload []byte) []GameMessage
    State(playerID int) any
    Type() GameType
    Start(playerIDs []int) []GameMessage
}
```

- `HandleAction` — process a player's game action (play card, draw, etc.), returns messages to route
- `State` — return the full game state visible to a specific player
- `Start` — initialize game with player IDs (not names); game engine has no knowledge of player names
- `GameMessage.Target` = -1 means broadcast to all players; otherwise unicast to that player ID
- Concrete implementations: `UnoGame`, `SkipBoGame`

### 3.6 WSC — WebSocket Client (internal/server/websocket.go)

Per-connection handler bridging WebSocket ↔ Hub.

```go
type WSC struct {
    ws    *websocket.Conn
    rm    *hub.RoomManager
    room  *hub.Room       // Room the user is in (nil if stray)
    state ClientState     // StateStray | StateBound
    playerName string     // Player name from join/create
    userID  int           // User session ID within room
    send  chan []byte     // Outgoing message buffer (shared with User)
}
```

**Key behaviors:**
- Two goroutines per connection: `writePump` + `readPump`
- Routes incoming JSON actions to handler methods
- On disconnect: removes User from Room, broadcasts player update
- Contains `playerName` to look up the Player struct (which holds the numeric ID used by the game engine)

---

## 4. WebSocket Protocol

Full spec in `WS_PROTOCOL.md`. Summary:

### Client → Server

| Action     | Payload                                    | Description                  |
|------------|--------------------------------------------|------------------------------|
| `create`   | `{name, game}`                             | Create room + join           |
| `join`     | `{name, roomID}`                           | Join existing room           |
| `leave`    | `{}`                                       | Leave current room           |
| `ready`    | `{}`                                       | Mark ready                   |
| `unready`  | `{}`                                       | Mark not ready               |
| `game`     | `{payload: {...}}`                         | Game-specific action         |

### Server → Client

| Action     | Payload                     | Trigger                          |
|------------|-----------------------------|----------------------------------|
| `joined`   | `{roomID}`                  | Successful create/join            |
| `left`     | `{}`                        | Successful leave                  |
| `ready`    | `{}`                        | Ready acknowledged                |
| `unready`  | `{}`                        | Unready acknowledged              |
| `players`  | `{players: [...]}`          | Player list changed (broadcast)   |
| `start`    | `{}`                        | All players ready (broadcast)     |
| `error`    | `{error: "reason"}`         | Any error                         |

---

## 5. Implementation Roadmap

### Phase 0: Fix Existing Refactoring TODOs (immediate)

1. **Remove `WSC.user` pointer** — WSC currently holds `user *hub.User` which duplicates state.
   - New approach: WSC doesn't store User directly; instead, WSC stores only `room *hub.Room` and later retrieves User via `room.GetUserBy...()` or similar.
   - Actually, better: WSC knows its **player name** and can look up state through the Room.

2. **Make `handleCreate` call `handleJoin`** — deduplicate room-joining logic.
   - `handleCreate` → create room → call `handleJoin` with the new room ID.

### Phase 1: Clean Architecture (structural improvements)

1. **Make `WSC` not store a `*hub.User`** — Instead, WSC stores `playerName string` and looks up state through `wsc.room`. This removes the stale pointer problem.

2. **Add `GetUserByPlayerName` to Room** — needed for WSC to find the User for a given player.

3. **Return game state on `joined`** — Include initial game state in the `joined` response (or provide a `status` action).

4. **Add `status` action** — Client asks for full game state mid-game (useful for reconnection).

### Phase 2: Game Engine — Uno

Port the full Uno implementation from `old/games/uno.cpp` to Go:

1. **Card model** — enums for color, symbol, card
2. **Deck** — generic deck with shuffle, draw, reset (from discard recycling)
3. **Game state** — draw pile, discard pile, players' hands, current turn, direction, draw counter
4. **Rules** — can_play, play, draw, skip, reverse, draw-two, wild, wild-draw-four
5. **Action handlers** — play, draw, sort (hand reordering), wild (color choice), keep
6. **State serialization** — produce player-specific game state (own hand visible, others show count)
7. **Broadcasting** — game events (play, draw, uno, over, next turn)

### Phase 3: Game Engine — Skip-Bo

Port Skip-Bo from the C++ stubs (currently empty).

### Phase 4: Polish & Production Readiness

1. **Configurable game settings** — initial hand size, play-after-draw, etc. (passed at room creation)
2. **Room cleanup** — delete empty rooms after timeout
3. **Graceful shutdown** — signal handling, drain connections
4. **Logging** — structured logging
5. **Metrics** — active rooms, connections, game counts

---

## 6. Detailed Uno Game Design (ported from C++)

### Cards

```go
type Color int
const (
    Red Color = iota
    Yellow
    Green
    Blue
    Black
)

type Symbol int
const (
    Zero Symbol = iota
    One, Two, Three, Four, Five, Six, Seven, Eight, Nine
    Skip
    Reverse
    DrawTwo
    Wild
    WildDrawFour
)

type Card struct {
    Color  Color
    Symbol Symbol
}
```

### Deck

```go
type Deck struct {
    cards []Card
    rng   *rand.Rand
}

func NewDeck(cards []Card) *Deck    // Create + shuffle
func (d *Deck) Draw() Card          // Draw top card (panics if empty)
func (d *Deck) Reset(cards []Card)  // Replace cards + shuffle
func (d *Deck) Empty() bool
func (d *Deck) Size() int
```

### Game State

```go
type UnoGame struct {
    players      []*UnoPlayer
    drawPile     *Deck
    discardPile  []Card
    direction    int          // 1 or -1
    currentTurn  int          // index into players
    drawCounter  int          // accumulated draws from DrawTwo/WildDrawFour
    winner       int
    over         bool
    config       UnoConfig
}

type UnoPlayer struct {
    name string
    hand []Card
}

type UnoConfig struct {
    InitialHandSize  int   // default 7
    PlayAfterDraw    bool  // default true
    AggregateDraws   bool  // default true
    BlackOnBlack     bool  // default true
}
```

### Rules Implementation

| Rule | Logic |
|------|-------|
| **can_play(card)** | Match color OR symbol of top discard; wilds always playable (unless black-on-black restricts); check draw_counter rules |
| **play(card)** | Place on discard pile, apply effect (skip=skip next, reverse=flip direction, draw_two/counter, wild draw four) |
| **draw** | Draw from deck; if deck empty, recycle discard pile (keep top). If `play_after_draw` and drawn card is playable, give option to play it |
| **skip** | Direction becomes 2 or -2 (skip one player) |
| **reverse** | Multiply direction by -1 |
| **draw effects** | Increment draw_counter; player must draw that many cards (aggregated if consecutive draw cards played) |
| **wild** | Player chooses color; when played, card is recolored |
| **win condition** | Player empties hand; if last card, announce UNO |

### Turn Flow

```
currentPlayer plays/draws
    → apply effects
    → advance to next player
    → broadcast {"action":"next","turn":nextIndex}
    → broadcast {"action":"ack"}
```

### Game Events (broadcast)

| Event | Payload | Description |
|-------|---------|-------------|
| `play` | `{player, handIndex, card}` | Card was played |
| `draw` | `{player, count}` | Player drew cards |
| `uno` | `{player}` | Player has 1 card left |
| `over` | `{winner}` | Game ended |
| `next` | `{turn}` | Next player's turn |
| `ack` | `{}` | Ready for next action |
| `keep_or_play` | `{card}` | Option to play just-drawn card |
| `error` | `{reason}` | Invalid action |

---

## 7. Message Protocol — Full Specification

### Room Management

The game engine is NOT created when the room is created. Only the game type and config are stored. The game is instantiated when all players are ready and `Room.StartGame()` is called, ensuring a finalized player list before cards are dealt.

#### Create Room
```json
// Request
{"action":"create", "name":"Alice", "game":"uno"}
// Response
{"action":"joined", "roomID":"abc123"}
// Broadcast
{"action":"players", "players":[{"id":0, "name":"Alice", "points":0, "active":true, "ready":false}]}
```

#### Message Flow (Create → Start)

```
Client creates room ──► handleCreate() stores game type + config, no game instance
Client joins                  ──► handleJoin() adds player (assigned ID 0)
Client readies up             ──► AllReady() → Room.StartGame()
                                          │
                                    game.NewGame(type, config)   ◄── Game created here
                                    game.Start(playerIDs)        ◄── Deals cards, picks top card (uses IDs, not names)
                                          │
                                    Client fetches state via status action
```

#### Join Room
```json
// Request
{"action":"join", "name":"Alice", "roomID":"abc123"}
// Response (new player)
{"action":"joined", "roomID":"abc123"}
// Response (reconnecting)
{"action":"joined", "roomID":"abc123", "reconnected":true}
```

#### Leave Room
```json
// Request
{"action":"leave"}
// Response
{"action":"left"}
```

#### Ready / Unready
```json
// Request
{"action":"ready"}
// Response
{"action":"ready"}
// All ready triggers broadcast:
{"action":"start"}
```

#### Status (get game state)
```json
// Request
{"action":"status"}
// Response (during game)
{"action":"status", "state":"running", "game":{...}, "players":[...], "myTurn":true}
// Response (lobby)
{"action":"status", "state":"init", "players":[...], "roomID":"abc123"}
```

### Game Actions (Uno)

#### Play a Card
```json
// Request
{"action":"game", "payload":{"action":"play_card", "card":{...}, "hand_index":3}}
// Broadcast
{"action":"card_played", "player":0, "card":{...}, "hand_index":3}
```

#### Draw a Card
```json
// Request
{"action":"game", "payload":{"action":"draw_card"}}
// Response to player
{"action":"draw", "cards":[{...}]}
// Broadcast to others
{"action":"card_drawn", "player":0, "count":1}
```

#### Choose Wild Color
```json
// Request
{"action":"game", "payload":{"action":"wild", "index":3, "color":"r"}}
// (r=red, y=yellow, g=green, b=blue)
```

#### Sort Hand
```json
// Request
{"action":"game", "payload":{"action":"sort", "from":3, "to":1}}
```

#### Keep Drawn Card (skip play-after-draw)
```json
// Request
{"action":"game", "payload":{"action":"keep"}}
```

---

## 8. Codebase Structure (final target)

```
pd-core3/
├── cmd/
│   └── server/
│       └── main.go              # Entry point, flag parsing
├── internal/
│   ├── hub/
│   │   ├── room.go              # Room struct + methods
│   │   ├── room_manager.go      # RoomManager
│   │   ├── player.go            # Player entity
│   │   └── user.go              # User session
│   ├── game/
│   │   ├── game.go              # Game interface + factory
│   │   ├── deck.go              # Generic deck implementation
│   │   ├── uno.go               # Uno game engine
│   │   └── skipbo.go            # Skip-Bo game engine (future)
│   └── server/
│       ├── websocket.go         # WebSocket handler + WSC
│       └── server_test.go       # Integration test
├── old/                         # Archived C++ implementation
├── ARCHITECTURE.md              # This document
├── WS_PROTOCOL.md               # WebSocket protocol spec
├── go.mod
├── go.sum
└── .gitignore
```

---

## 9. Error Handling Strategy

All errors returned as:
```json
{"action":"error", "error":"error_code"}
```

| Error Code | When |
|------------|------|
| `invalid_json` | Failed to parse incoming message |
| `missing_action` | No `action` field in message |
| `unknown_action` | Unrecognized action string |
| `not_stray` | Player must not be in a room for this action |
| `not_bound` | Player must be in a room for this action |
| `missing_name` | No `name` field provided |
| `missing_game` | No `game` field provided |
| `invalid_game` | Unsupported game type |
| `missing_room_id` | No `roomID` field provided |
| `room_not_found` | Room ID does not exist |
| `name_taken` | Player name already active in room |
| `failed_to_connect` | Internal error binding user to player |
| `missing_payload` | No `payload` field in game action |
| `invalid_payload` | Payload cannot be serialized |

---

## 10. Concurrency Model

```
┌─────────────────────────────┐
│  Room.mu (RWMutex)           │  ← Protects players/users maps
│  RoomManager.mu (Mutex)      │  ← Protects rooms map
└─────────────────────────────┘

Per WebSocket Connection:
  ├── readPump goroutine  ──► reads WS messages, calls handleMessage
  └── writePump goroutine ◄── reads from send channel, writes to WS

User.Send channel (buffered, cap 256):
  ◄── Room.Broadcast / BroadcastOthers writes to it
  ──► writePump goroutine reads from it
  Non-blocking via TrySend() — drops message if channel full

Atomic:
  User.ready — atomic.Bool, lock-free reads
```

**Key principles:**
- Never hold a lock while sending on a channel
- Room methods acquire locks internally
- WSC never holds a room-level reference across message boundaries (reads happen in readPump)

---

## 11. Testing Strategy

| Layer | Test | Scope |
|-------|------|-------|
| **Hub** | Unit tests | Room, Player, User, RoomManager in isolation |
| **Server** | Integration tests | Full WebSocket round-trip with httptest |
| **Game** | Unit tests | Uno rules, edge cases, win conditions |

**Hub tests** — use `MockGame` (already exists), buffered channels for fake users.

**Server tests** — `httptest.NewServer` + real WebSocket connection, verify JSON responses.

**Uno tests** — construct game with known deck order, assert exact state transitions.

---

## 12. Future Considerations

1. **Authentication** — add token-based player identity (instead of just name)
2. **Rate limiting** — prevent spam actions per connection
3. **Replay / spectate** — record game events, allow late-joining spectators
4. **Bot players** — simple AI for solo practice
5. **Multiple server instances** — shared room state via Redis/pub-sub
6. **TLS** — WSS support with Let's Encrypt
7. **Docker** — containerized deployment
