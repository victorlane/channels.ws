package services

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"channels.ws/internal/config"
	"channels.ws/internal/database"
	"channels.ws/internal/models"
	"github.com/gorilla/websocket"
)

type WebSocketService struct {
	db          *database.DB
	config      *config.Config
	upgrader    websocket.Upgrader
	connections map[string][]*websocket.Conn
	mu          sync.RWMutex
}

func NewWebSocketService(db *database.DB, cfg *config.Config) *WebSocketService {
	return &WebSocketService{
		db:     db,
		config: cfg,
		upgrader: websocket.Upgrader{
			EnableCompression: true,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		connections: make(map[string][]*websocket.Conn),
	}
}

type SessionData struct {
	ID           int64
	SubdomainID  string
	ClientIP     string
	Messages     []string // Temporary storage for echoing back
	MessageCount int
	TotalPayload int64
	StartTime    time.Time
}

func (ws *WebSocketService) UpgradeConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	return ws.upgrader.Upgrade(w, r, nil)
}

func (ws *WebSocketService) HandleConnection(conn *websocket.Conn, subdomainID, clientIP string) error {

	// Create session in database
	sessionID, err := ws.db.CreateSession(subdomainID, clientIP, time.Now())
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		return err
	}

	session := &SessionData{
		ID:           sessionID,
		SubdomainID:  subdomainID,
		ClientIP:     clientIP,
		Messages:     make([]string, 0),
		MessageCount: 0,
		TotalPayload: 0,
		StartTime:    time.Now(),
	}

	// Add connection to tracking
	ws.addConnection(subdomainID, conn)
	defer ws.removeConnection(subdomainID, conn)

	log.Printf("Client connected from %s (subdomain: %s, session: %d)", clientIP, subdomainID, sessionID)

	// Handle the session
	return ws.handleSession(conn, session)
}

func (ws *WebSocketService) handleSession(conn *websocket.Conn, session *SessionData) error {
	var mu sync.Mutex
	done := make(chan bool, 1)

	// Start timeout
	go func() {
		time.Sleep(ws.config.SessionTimeout)
		done <- true
	}()

	// Read messages
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				conn.SetReadDeadline(time.Now().Add(ws.config.ReadTimeout))

				_, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("WebSocket error: %v", err)
					}
					return
				}

				// Store message temporarily for echoing back
				mu.Lock()
				session.Messages = append(session.Messages, string(message))
				session.MessageCount++
				session.TotalPayload += int64(len(message))
				mu.Unlock()
			}
		}
	}()

	// Wait for timeout
	<-done

	endTime := time.Now()
	duration := endTime.Sub(session.StartTime)

	mu.Lock()
	messageCount := session.MessageCount
	totalPayload := session.TotalPayload
	messages := make([]string, len(session.Messages)) // Copy messages for response

	copy(messages, session.Messages)
	mu.Unlock()

	if err := ws.db.SaveMetadata(session.ID, session.SubdomainID, messageCount, totalPayload); err != nil {
		log.Printf("Failed to save metadata: %v", err)
	}

	// Send response with message content (but don't persist messages to database)
	response := models.Response{
		Messages:  messages, // Echo back the messages received during session
		Timestamp: endTime,
		Duration:  duration.String(),
	}

	if err := conn.WriteJSON(response); err != nil {
		log.Printf("Failed to send response: %v", err)
		return err
	}

	if err := ws.db.EndSession(session.ID, endTime, duration.String(), messageCount); err != nil {
		log.Printf("Failed to end session in database: %v", err)
	}

	log.Printf("Sent response with %d messages collected over %v (session: %d)", messageCount, duration, session.ID)
	return nil
}

func (ws *WebSocketService) addConnection(subdomainID string, conn *websocket.Conn) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.connections[subdomainID] = append(ws.connections[subdomainID], conn)
}

func (ws *WebSocketService) removeConnection(subdomainID string, conn *websocket.Conn) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	connections := ws.connections[subdomainID]
	for i, c := range connections {
		if c == conn {
			ws.connections[subdomainID] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	if len(ws.connections[subdomainID]) == 0 {
		delete(ws.connections, subdomainID)
	}
}

func (ws *WebSocketService) GetActiveConnections(subdomainID string) int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return len(ws.connections[subdomainID])
}

func ExtractSubdomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		return parts[0]
	}
	return ""
}
