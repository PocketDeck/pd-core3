type Game interface {
	Start(room *Room)
	HandleAction(userID int, payload json.RawMessage)
	State() any
}
