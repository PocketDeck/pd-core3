package main

import (
	"flag"
	"log"
	"net/http"

	"PocketDeck/pd-core3/internal/hub"
	"PocketDeck/pd-core3/internal/server"
)

func main() {
	host := flag.String("host", "localhost", "host")
	port := flag.String("port", "8080", "port")
	flag.Parse()

	rm := hub.NewRoomManager()

	http.Handle("/", server.NewWebSocketHandler(rm))

	log.Println("Starting server on", *host+":"+*port)
	log.Fatal(http.ListenAndServe(*host+":"+*port, nil))
}
