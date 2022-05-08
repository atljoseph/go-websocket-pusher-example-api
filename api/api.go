// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"go-websocket-pusher-example-api/handlers"
	"go-websocket-pusher-example-api/websocket"

	"github.com/gorilla/mux"
)

type API struct {
	WebsocketServer *websocket.Server
}

func NewAPI(wss *websocket.Server) *API {
	return &API{
		WebsocketServer: wss,
	}
}

// Run runs the API alongside a websocket server.
func (api *API) Run(addr string) error {

	// Start up a websocket server in the background.
	// We'll feed events to it based on events traversing the HTTP server.
	go api.WebsocketServer.Run()

	// Boot up a new HTTP router & add routed handlers for various events & for initiating websocket connections.
	router := mux.NewRouter()
	router.HandleFunc("/", handlers.HomeHttpHandler).Methods("GET")
	router.HandleFunc("/chat-message", func(w http.ResponseWriter, r *http.Request) {
		handlers.SendChatMessageHandler(api.WebsocketServer, w, r)
	}).Methods("POST")
	router.HandleFunc("/system-message", func(w http.ResponseWriter, r *http.Request) {
		handlers.SendSystemMessageHandler(api.WebsocketServer, w, r)
	}).Methods("POST")
	router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handlers.InitializeWebsocketHandler(api.WebsocketServer, w, r)
	}).Methods("GET")

	// Finally, start the HTTP server.
	err := http.ListenAndServe(addr, router)
	if err != nil {
		return err
	}
	return nil
}

// TODO: api has a heartbeat. on the heartbeat, it checks things. if something found, notifies on websocket.
