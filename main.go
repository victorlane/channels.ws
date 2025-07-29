package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"channels.ws/internal/config"
	"channels.ws/internal/database"
	"channels.ws/internal/handlers"
	"channels.ws/internal/services"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting channels.ws server on port %s", cfg.Port)

	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	defer db.Close()

	wsService := services.NewWebSocketService(db, cfg)

	h := handlers.New(wsService, cfg)

	http.HandleFunc("/", h.WebSocket)
	http.HandleFunc("/health", h.Health)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", cfg.Port)
		if err := http.ListenAndServe(cfg.Port, nil); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-sigChan
	log.Println("Shutting down server...")
	log.Println("Server stopped")
}
