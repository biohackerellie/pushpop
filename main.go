package main

import (
	"api/internal/chat"
	"api/internal/lib/logger"
	"context"
	"net/http"
	"os"
	"os/signal"
)

func main() {
	log := logger.New()

	hub := chat.NewHub()
	go hub.Run()

	// Register routes
	http.HandleFunc("/trigger", chat.HandleTrigger(hub))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		chat.ServeWs(hub, w, r)
	})
	// Start the server
	server := &http.Server{
		Addr: "0.0.0.0:8945",
	}

	// Graceful shutdown context
	shutdownCtx, stop := context.WithCancel(context.Background())
	defer stop()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server error", "err", err)
		}
	}()
	// Wait for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	log.Info("Shutting down server...")

	// Stop accepting new requests and clean up
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server shutdown failed", "err", err)
	}
	log.Info("Server gracefully stopped")
}
