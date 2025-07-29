package handlers

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"channels.ws/internal/config"
	"channels.ws/internal/middleware"
	"channels.ws/internal/services"
	"lukechampine.com/blake3"
)

type Handlers struct {
	wsService  *services.WebSocketService
	config     *config.Config
	spamFilter *middleware.SpamFilter
}

func hash(in string) string {
	sum := blake3.Sum256([]byte(in))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func New(wsService *services.WebSocketService, cfg *config.Config) *Handlers {
	var spamFilter = middleware.NewSpamFilterWithConfig(
		cfg.MaxRequestsPerSecond,
		cfg.RateLimitWindowDuration,
		cfg.CooldownDuration,
		cfg.MaxCooldownDuration,
		cfg.MaxViolations,
	)

	return &Handlers{
		wsService:  wsService,
		config:     cfg,
		spamFilter: spamFilter,
	}
}

func (h *Handlers) WebSocket(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxPayloadSize)

	// Use the spam filter's CheckRequest method for all protection checks
	if h.spamFilter != nil {
		allowed, _, errorMsg, statusCode := h.spamFilter.CheckRequest(r)
		if !allowed {
			http.Error(w, errorMsg, statusCode)
			return
		}
	}

	clientIP := middleware.NormalizeIP(getClientIP(r))
	clientIP = hash(clientIP) // Hash the client IP for privacy

	subdomainID := services.ExtractSubdomain(r.Host)
	if subdomainID == "" {
		log.Printf("Request from main domain (no subdomain) - identifier: %s", clientIP)
	} else {
		log.Printf("Request from subdomain: %s - identifier: %s", subdomainID, clientIP)
	}

	conn, err := h.wsService.UpgradeConnection(w, r)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}
	defer conn.Close()

	if err := h.wsService.HandleConnection(conn, subdomainID, clientIP); err != nil {
		log.Printf("Error handling WebSocket connection: %v", err)
	}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.config.MaxPayloadSize)

	if h.spamFilter != nil {
		allowed, _, errorMsg, statusCode := h.spamFilter.CheckRequest(r)
		if !allowed {
			http.Error(w, errorMsg, statusCode)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":  "healthy",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": "1.0.0",
		"service": "channels-websocket",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode health response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.Index(xff, ","); comma != -1 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}

	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}
