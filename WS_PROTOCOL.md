# WebSocket JSON Protocol

## Connection
Connect to: `ws://host:port/ws`

---

## Client → Server Messages

### Create Room
Create a new game room and join as the first player

**Request:**
```json
{
  "action": "create",
  "name": "PlayerName",
  "game": "uno"
}
```

**Responses:**
- `joined` - Successfully created and joined the room
  ```json
  {
    "action": "joined",
    "roomID": "abc123"
  }
  ```
- `navigate` - Navigate to lobby
  ```json
  {
    "action": "navigate",
    "page": "/lobby"
  }
  ```
- `players` - (Broadcast) Player list update
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_stray|missing_name|missing_game|invalid_game"
  }
  ```

---

### Join Room
Join an existing game room

**Request:**
```json
{
  "action": "join",
  "name": "PlayerName",
  "roomID": "abc123"
}
```

**Responses:**
- `joined` - Successfully joined the room
  ```json
  {
    "action": "joined",
    "roomID": "abc123"
  }
  ```
- `navigate` - Navigate to lobby
  ```json
  {
    "action": "navigate",
    "page": "/lobby"
  }
  ```
- `players` - (Broadcast) Player list update
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_stray|missing_name|missing_room_id|room_not_found|name_taken|failed_to_connect"
  }
  ```

**Notes:**
- **Reconnect**: If the name exists and the player is inactive, you automatically reconnect!
- **Unique names**: Names are unique per room - error "name_taken" if name is already active

---

### Ready
Mark yourself as ready to start the game

**Request:**
```json
{ "action": "ready" }
```

**Responses:**
- `ready` - Acknowledgment
  ```json
  { "action": "ready" }
  ```
- `players` - (Broadcast) Player list update
- `start` - (Broadcast, if all players ready) Game started
  ```json
  { "action": "start" }
  ```
- `navigate` - (Broadcast) Navigate to game page
  ```json
  {
    "action": "navigate",
    "page": "/games/uno"
  }
  ```
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_bound"
  }
  ```

---

### Unready
Mark yourself as not ready

**Request:**
```json
{ "action": "unready" }
```

**Responses:**
- `unready` - Acknowledgment
  ```json
  { "action": "unready" }
  ```
- `players` - (Broadcast) Player list update
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_bound"
  }
  ```

---

### Leave Room
Leave the current game room

**Request:**
```json
{ "action": "leave" }
```

**Responses:**
- `left` - Acknowledgment
  ```json
  { "action": "left" }
  ```
- `navigate` - Navigate to home
  ```json
  {
    "action": "navigate",
    "page": "/"
  }
  ```
- `players` - (Broadcast) Player list update
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_bound"
  }
  ```

---

### Status
Get current room and game state

**Request:**
```json
{ "action": "status" }
```

**Responses:**
- `status` - Current room and game state
  ```json
  {
    "action": "status",
    "roomID": "abc123",
    "players": [
      {
        "name": "Alice",
        "points": 0,
        "active": true,
        "ready": false
      }
    ],
    "game": null
  }
  ```
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_bound"
  }
  ```

---

### Game Action
Send a game-specific action

**Request:**
```json
{
  "action": "game",
  "payload": { "your": "game data" }
}
```

**Responses:**
- `error` - Error occurred
  ```json
  {
    "action": "error",
    "error": "not_bound|missing_payload|invalid_payload"
  }
  ```

---

## Server → Client Messages (Unsolicited)

### Player List Update
**Broadcast automatically when:**
- A player joins
- A player leaves
- A player's ready status changes
- A player disconnects unexpectedly

```json
{
  "action": "players",
  "players": [
    {
      "name": "Alice",
      "points": 0,
      "active": true,
      "ready": false
    }
  ]
}
```

### Game Started
Broadcast when all players are ready

```json
{ "action": "start" }
```

### Navigate
Sent to navigate the client to a different page

```json
{
  "action": "navigate",
  "page": "/lobby"
}
```

**Note:** The client never decides where to navigate. The server always sends `navigate` when a route change is needed:
- After `joined` → navigates to `/lobby`
- After `start` → navigates to `/games/uno` (or whatever game was configured)
- After `left` → navigates to `/`
