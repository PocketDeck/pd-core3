package game

type Game interface {
	HandleAction(userID int, payload []byte)
	State(userID int) any
}