package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	p "github.com/biohackerellie/pushpop"
)

func main() {

	var levelVar slog.LevelVar
	level, _ := os.LookupEnv("LOG_LEVEL")
	if err := levelVar.UnmarshalText([]byte(level)); err != nil {
		levelVar.Set(slog.LevelError)
	}
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: &levelVar,
	})
	log := slog.New(logHandler)

	hub := p.NewHub(log)
	go hub.Run()
	// Register routes
	http.HandleFunc("/trigger", p.HandleTrigger(hub))
	http.HandleFunc("/ws", p.ServeWs(hub))
	// Start the server
	server := &http.Server{
		Addr: "0.0.0.0:8945",
	}

	// Graceful shutdown context
	shutdownCtx, stop := context.WithCancel(context.Background())
	defer stop()

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server error", "err", err)
			panic(err)
		}
	}()
	// Wait for shutdown signal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	log.Info("Shutting down server...")

	// Stop accepting new requests and clean up
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("Server shutdown failed", "err", err)
	}
	log.Info("Server gracefully stopped")
}
