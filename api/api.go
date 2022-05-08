package api

import (
	"context"
	"net/http"

	"go-websocket-pusher-example-api/handlers"
	"go-websocket-pusher-example-api/websocket"

	"github.com/gorilla/mux"
)

type API struct {
	*http.Server
}

func NewAPI() *API {
	return &API{}
}

func (api *API) Shutdown(ctx context.Context) error {
	if api.Server != nil {
		return api.Server.Shutdown(ctx)
	}
	return nil
}

// Run runs the API alongside a websocket server.
func (api *API) Run(ctx context.Context, addr string, wss *websocket.Server) error {

	// Boot up a new HTTP router & add routed handlers for various events & for initiating websocket connections.
	router := mux.NewRouter()
	router.HandleFunc("/", handlers.HomeHttpHandler).Methods("GET")
	router.HandleFunc("/chat-message", func(w http.ResponseWriter, r *http.Request) {
		handlers.SendChatMessageHandler(wss, w, r)
	}).Methods("POST")
	router.HandleFunc("/system-message", func(w http.ResponseWriter, r *http.Request) {
		handlers.SendSystemMessageHandler(wss, w, r)
	}).Methods("POST")
	router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handlers.InitializeWebsocketHandler(wss, w, r)
	}).Methods("GET")
	router.NotFoundHandler = router.NewRoute().HandlerFunc(http.NotFound).GetHandler()

	// Finally, start the HTTP server.
	api.Server = &http.Server{
		Addr:    addr,
		Handler: router,
	}
	return api.Server.ListenAndServe()
}
