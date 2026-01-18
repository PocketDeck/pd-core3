package main

import (
	"flag"
	"log"
	"net/http"

	"golang.org/x/net/websocket"
)

func wsHandler(ws *websocket.Conn) {
	defer ws.Close()

	log.Println("New connection")

	for {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			log.Println("Error receiving message:", err)
			break
		}
		log.Println("Received message:", msg)

		reply := "Hello " + msg
		err = websocket.Message.Send(ws, reply)
		if err != nil {
			log.Println("Error sending message:", err)
			break
		}
	}
}

func main() {
	host := flag.String("host", "localhost", "host")
	port := flag.String("port", "8080", "port")
	flag.Parse()

	http.Handle("/ws", websocket.Handler(wsHandler))

	log.Println("Starting server on", *host+":"+*port)
	log.Fatal(http.ListenAndServe(*host+":"+*port, nil))
}