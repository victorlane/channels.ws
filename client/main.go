package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := "ws://localhost:8080/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected to WebSocket server")
	fmt.Println("Sending test messages for 8 seconds...")

	// Send test messages for 8 seconds
	go func() {
		for i := 1; i <= 8; i++ {
			message := fmt.Sprintf("Test message #%d at %s", i, time.Now().Format("15:04:05"))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
				log.Printf("Failed to send message: %v", err)
				return
			}
			fmt.Printf("Sent: %s\n", message)
			time.Sleep(1 * time.Second)
		}
	}()

	// Read responses
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}

		if messageType == websocket.TextMessage {
			fmt.Printf("Received: %s\n", string(message))
		} else {
			fmt.Printf("Received JSON response: %s\n", string(message))
		}
	}
}
