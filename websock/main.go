package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"code.google.com/p/go.net/websocket"
)

func main() {
	http.Handle("/", websocket.Handler(CounterServer))
	if err := http.ListenAndServe(":5603", nil); err != nil {
		log.Fatalln(err)
	}
}

func CounterServer(ws *websocket.Conn) {
	go func() {
		var i int
		for _ = range time.Tick(time.Second) {
			if err := websocket.Message.Send(ws, strconv.Itoa(i)); err != nil {
				log.Fatalln(err)
			}
			i++
		}
	}()
	for {
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			log.Fatalln(err)
		}
		log.Println("received:", msg)
	}
}
