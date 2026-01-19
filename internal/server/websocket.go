package server

import (
	"log"
	"net/http"
	"sync"

	"PocketDeck/pd-core3/internal/hub"
	"golang.org/x/net/websocket"
)

type WSC struct {
	ws *websocket.Conn
	ID int

	bcast chan byte
	action *chan byte
}

func NewWebSocketHandler() http.Handler {
	return websocket.Handler(func (ws *websocket.Conn) {
		defer ws.Close()

		log.Println("New connection; Creating client")

		client := &WSC{ws:ws, action:nil}
		var wg sync.WaitGroup

		wg.Go(client.readPump)
		wg.Go(client.writePump)

		wg.Wait()
		close(client.bcast)
		ws.Close()
	})
}

func (wsc *WSC) writePump() {
	for msg := range wsc.bcast {
		err := websocket.Message.Send(wsc.ws, msg)

		if err != nil {
			log.Println("Error sending message:", err)
			break
		}
	}
}

func (wsc *WSC) readPump() {
	for {
		var msg string
		err := websocket.Message.Receive(wsc.ws, &msg)
		if err != nil {
			log.Println("Error receiving message:", err)
			break
		}

		wsc.handleAction(msg)
	}
}

func (wsc *WSC) handleAction(msg string) {
	;
}
