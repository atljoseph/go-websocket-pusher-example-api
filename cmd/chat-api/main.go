package main

import (
	"flag"

	"go-websocket-pusher-example-api/api"
	"go-websocket-pusher-example-api/websocket"
)

func main() {

	addr := flag.String("addr", ":8080", "http service address")
	flag.Parse()
	chatAPI := api.NewAPI(websocket.NewServer())
	chatAPI.Run(*addr)
}
