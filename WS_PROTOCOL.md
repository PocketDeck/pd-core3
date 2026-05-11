# WebSocket JSON Protocol

## Connection
Connect to: `ws://host:port/ws`

> **Internal note:** The game engine is NOT created when a room is created — only the game type and config are stored. The game is instantiated when all players are ready and the server calls `Room.StartGame()`. This ensures a finalized player list before cards are dealt. Pre-start game actions (`play_card`, `draw_card`, etc.) will be silently ignored.

---

## Client → Server Messages

### Create Room
Create a new game room and join as the first player

**Request:**
```json
{
  "action": "create",
  "name": "PlayerName",
  "game": "uno",
  "config": {
    "cardsPerPlayer": 7,
    "pointsToWin": 500,
    "playAfterDraw": true,
    "aggregateDraws": true,
    "blackOnBlack": true
  }
}
```

`config` is optional — defaults shown above.
Individual fields may be omitted to use defaults.

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
  "payload": { "action": "draw_card" }
}
```

**Responses:**
- `error` - Error occurred (not_bound, missing_payload, invalid_payload)
  ```json
  { "action": "error", "error": "not_bound" }
  ```
- Game-specific messages (see Uno section below)

---

## Uno Game Protocol

### Game State (on start + via status)

When game starts (`start` broadcast), the game broadcasts a `game_state` message followed by unicast `hand` messages per player:

```json
{
  "action": "game_state",
  "state": "playing",
  "turn": "Alice",
  "direction": 1,
  "drawPile": 94,
  "topCard": { "color": "red", "kind": "number", "value": 3 },
  "players": [
    { "name": "Alice", "card_count": 7 },
    { "name": "Bob", "card_count": 7 }
  ]
}
```

Followed by unicast to each player:
```json
{ "hand": [ { "color": "red", "kind": "number", "value": 5 }, ... ] }
```

The same data is available via `status` response under the `game` field.

### Actions (sent as `payload` in game action)

**Draw a card:**
```json
{ "action": "draw_card" }
```

Response (unicast to player):
```json
{ "action": "draw", "cards": [ { "color": "blue", "kind": "number", "value": 7 } ] }
```

Broadcast to others:
```json
{ "action": "card_drawn", "player": "Alice", "count": 1 }
```

If `playAfterDraw` is enabled and the drawn card can be played:
```json
{ "action": "keep_or_play", "card": [{...}], "played_at_index": 7 }
```

**Play a card:**
```json
{
  "action": "play_card",
  "card": { "color": "red", "kind": "number", "value": 5 },
  "hand_index": 0,
  "wildColor": "blue"
}
```
`hand_index` is optional (used when you must play the just-drawn card).  
`wildColor` is required for wild/wilddraw4 cards.

Response (broadcast):
```json
{ "action": "card_played", "player": "Alice", "card": {...}, "hand_index": 0 }
```

**Call Uno:**
```json
{ "action": "call_uno" }
```
Response (broadcast):
```json
{ "action": "uno_called", "player": "Alice" }
```

**Declare wild color:**
```json
{ "action": "declare_color", "color": "blue", "hand_index": 3 }
```
Response (broadcast):
```json
{ "action": "color_declared", "player": "Alice", "color": "blue" }
```

### Event Messages (broadcast)

**Turn change:**
```json
{ "action": "turn", "player": "Bob" }
```

**Player skipped (Skip card or 2-player Reverse):**
```json
{ "action": "player_skipped", "player": "Bob" }
```

**Direction reversed (3+ players):**
```json
{ "action": "direction_reversed", "direction": -1 }
```

**Draw penalty (Draw Two or Wild Draw Four):**
```json
{ "action": "draw_penalty", "player": "Bob", "count": 2 }
```

**Uno (player has 1 card remaining after play):**
```json
{ "action": "uno", "player": "Alice" }
```

**Game over:**
```json
{ "action": "game_over", "winner": "Alice" }
```

### Error messages (unicast to the player)
```json
{ "action": "error", "error": "not_your_turn" }
```

Possible errors: `not_your_turn`, `card_not_in_hand`, `cannot_play_card`, `must_declare_color`, `missing_card`, `must_play_drawn_card`, `invalid_action`, `unknown_game_action`, `deck_empty`, `not_bound`, `missing_payload`, `invalid_payload`

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
