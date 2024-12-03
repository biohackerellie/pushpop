package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"

	p "github.com/biohackerellie/pushpop"
)

func main() {

	hub := p.NewHub()
	go hub.Run()

	// Register routes
	http.HandleFunc("/trigger", p.HandleTrigger(hub))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		p.ServeWs(hub, w, r)
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
	log.Print("Shutting down server...")

	// Stop accepting new requests and clean up
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal("Server shutdown failed", "err", err)
	}
	log.Print("Server gracefully stopped")
}
