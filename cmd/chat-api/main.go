package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-websocket-pusher-example-api/api"
	"go-websocket-pusher-example-api/websocket"

	"github.com/sirupsen/logrus"
)

func main() {

	addr := flag.String("addr", ":8080", "http service address")
	flag.Parse()

	// Get conext and cancel from the background, then start up the servers.
	// ctx, cancel := context.WithTimeout(context.Background(), time.Duration(15*time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start up a websocket server in the background.
	// We'll feed events to it based on events traversing the HTTP server.
	websocketServer := websocket.NewServer()
	go websocketServer.Run(ctx)

	// Start up the http server, too.
	// It can feed stuff into the websocket server.
	chatAPI := api.NewAPI()
	go func() {
		if err := chatAPI.Run(ctx, *addr, websocketServer); err != nil {
			logrus.WithError(err).Fatal("failed to start API server to accompany websocket server")
		}
	}()

	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C) or SIGKILL
	// SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	sigquit := make(chan os.Signal, 1)
	signal.Notify(sigquit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block & Listen here for shutdown events.
	defer func() {
		logrus.Info("Gracefully shutting down server...")
		cancel()
		_ = chatAPI.Shutdown(ctx)

		logrus.Info("Shutting down")
		os.Exit(0)
	}()
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case sig := <-sigquit:
			logrus.Infof("caught sig: %+v", sig)
			return
		case <-ctx.Done():
			logrus.Infof("context cancelled; quitting server")
			return
		case <-ticker.C:
			// Loop as often as every second.
		}
	}
}
