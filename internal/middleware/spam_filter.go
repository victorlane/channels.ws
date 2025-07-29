package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"channels.ws/internal/models"
)

type RateLimitTier int

const (
	TierNormal RateLimitTier = iota
	TierWarning
	TierStrictCooldown
	TierBlocked
)

type ClientState struct {
	RequestCount    int
	LastRequestTime time.Time
	WindowStart     time.Time
	CurrentTier     RateLimitTier
	CooldownUntil   time.Time
	ViolationCount  int
	TotalRequests   int64
	FirstSeen       time.Time
	mu              sync.RWMutex
}

type SpamFilter struct {
	clients    map[string]*ClientState
	blockedIPs map[string]time.Time
	mu         sync.RWMutex

	WindowDuration      time.Duration
	MaxRequestsPerSec   int
	CooldownDuration    time.Duration
	MaxCooldownDuration time.Duration
	MaxViolations       int
	CleanupInterval     time.Duration
}

// NewSpamFilter creates a new spam filter instance
func NewSpamFilter() *SpamFilter {
	sf := &SpamFilter{
		clients:             make(map[string]*ClientState),
		blockedIPs:          make(map[string]time.Time),
		WindowDuration:      1 * time.Second,
		MaxRequestsPerSec:   10,
		CooldownDuration:    10 * time.Second,
		MaxCooldownDuration: 1 * time.Hour,
		MaxViolations:       5,
		CleanupInterval:     5 * time.Minute,
	}

	// Start cleanup goroutine
	go sf.periodicCleanup()

	return sf
}

// NewSpamFilterWithConfig creates a new spam filter instance with custom configuration
func NewSpamFilterWithConfig(maxRequestsPerSec int, windowDuration, cooldownDuration, maxCooldownDuration time.Duration, maxViolations int) *SpamFilter {
	sf := &SpamFilter{
		clients:             make(map[string]*ClientState),
		blockedIPs:          make(map[string]time.Time),
		WindowDuration:      windowDuration,
		MaxRequestsPerSec:   maxRequestsPerSec,
		CooldownDuration:    cooldownDuration,
		MaxCooldownDuration: maxCooldownDuration,
		MaxViolations:       maxViolations,
		CleanupInterval:     5 * time.Minute,
	}

	// Start cleanup goroutine
	go sf.periodicCleanup()

	return sf
}

// IsBlocked checks if an IP is currently blocked
func (sf *SpamFilter) IsBlocked(ip string) bool {
	sf.mu.RLock()
	defer sf.mu.RUnlock()

	if blockedUntil, exists := sf.blockedIPs[ip]; exists {
		if time.Now().Before(blockedUntil) {
			return true
		}
		// Block expired, remove it
		delete(sf.blockedIPs, ip)
	}

	return false
}

// BlockIP permanently blocks an IP address
func (sf *SpamFilter) BlockIP(ip string) {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Block for a very long time (effectively permanent)
	sf.blockedIPs[ip] = time.Now().Add(365 * 24 * time.Hour)
	log.Printf("IP %s has been permanently blocked due to excessive violations", ip)
}

// CheckRequest performs all protection checks: bans, bad IPs, and rate limiting
func (sf *SpamFilter) CheckRequest(r *http.Request) (allowed bool, tier RateLimitTier, errorMsg string, statusCode int) {
	clientIP := NormalizeIP(getClientIP(r))

	// First check if IP is already blocked (cheapest check)
	if sf.IsBlocked(clientIP) {
		log.Printf("Blocked connection from already blocked IP: %s", clientIP)
		return false, TierBlocked, "IP permanently blocked", http.StatusForbidden
	}

	// Then check if it's a bad IP (external API call - more expensive)
	if isBadIP(clientIP) {
		log.Printf("Blocked connection from bad IP (external check): %s", clientIP)
		sf.BlockIP(clientIP)
		return false, TierBlocked, "Forbidden", http.StatusForbidden
	}

	// Finally check rate limits
	allowed, tier = sf.CheckRateLimit(clientIP)
	if !allowed {
		tierName := GetTierName(tier)
		log.Printf("Rate limit exceeded for IP %s (tier: %s)", clientIP, tierName)

		switch tier {
		case TierWarning:
			return false, tier, "Rate limit exceeded - temporary cooldown", http.StatusTooManyRequests
		case TierStrictCooldown:
			return false, tier, "Rate limit exceeded - extended cooldown", http.StatusTooManyRequests
		case TierBlocked:
			return false, tier, "IP permanently blocked", http.StatusForbidden
		default:
			return false, tier, "Rate limit exceeded", http.StatusTooManyRequests
		}
	}

	return true, tier, "", 0
}

// CheckRateLimit checks if a request should be allowed and updates client state
func (sf *SpamFilter) CheckRateLimit(ip string) (allowed bool, tier RateLimitTier) {
	now := time.Now()

	if sf.IsBlocked(ip) {
		return false, TierBlocked
	}

	sf.mu.Lock()
	defer sf.mu.Unlock()

	client, exists := sf.clients[ip]
	if !exists {
		client = &ClientState{
			FirstSeen:       now,
			WindowStart:     now,
			LastRequestTime: now,
			CurrentTier:     TierNormal,
		}
		sf.clients[ip] = client
	}

	client.mu.Lock()
	defer client.mu.Unlock()

	// Check if client is in cooldown
	if now.Before(client.CooldownUntil) {
		return false, client.CurrentTier
	}

	// Reset window if enough time has passed
	if now.Sub(client.WindowStart) >= sf.WindowDuration {
		client.WindowStart = now
		client.RequestCount = 0
	}

	client.RequestCount++
	client.TotalRequests++
	client.LastRequestTime = now

	// Check rate limit
	if client.RequestCount > sf.MaxRequestsPerSec {
		return sf.handleViolation(client, ip)
	}

	return true, client.CurrentTier
}

func (sf *SpamFilter) handleViolation(client *ClientState, ip string) (bool, RateLimitTier) {
	client.ViolationCount++

	cooldownDuration := sf.CooldownDuration * time.Duration(1<<uint(client.ViolationCount-1))
	if cooldownDuration > sf.MaxCooldownDuration {
		cooldownDuration = sf.MaxCooldownDuration
	}

	client.CooldownUntil = time.Now().Add(cooldownDuration)
	switch {
	case client.ViolationCount >= sf.MaxViolations:
		client.CurrentTier = TierBlocked
		sf.BlockIP(ip)
		log.Printf("IP %s exceeded max violations (%d), permanently blocked", ip, client.ViolationCount)
		return false, TierBlocked

	case client.ViolationCount >= 3:
		client.CurrentTier = TierStrictCooldown
		log.Printf("IP %s in strict cooldown (violation %d), cooldown: %v", ip, client.ViolationCount, cooldownDuration)
		return false, TierStrictCooldown

	case client.ViolationCount >= 1:
		client.CurrentTier = TierWarning
		log.Printf("IP %s rate limit exceeded (violation %d), cooldown: %v", ip, client.ViolationCount, cooldownDuration)
		return false, TierWarning

	default:
		return false, TierNormal
	}
}

func (sf *SpamFilter) GetClientStats(ip string) *ClientState {
	sf.mu.RLock()
	defer sf.mu.RUnlock()

	if client, exists := sf.clients[ip]; exists {
		client.mu.RLock()
		defer client.mu.RUnlock()

		// Return a copy to avoid race conditions
		return &ClientState{
			RequestCount:    client.RequestCount,
			LastRequestTime: client.LastRequestTime,
			WindowStart:     client.WindowStart,
			CurrentTier:     client.CurrentTier,
			CooldownUntil:   client.CooldownUntil,
			ViolationCount:  client.ViolationCount,
			TotalRequests:   client.TotalRequests,
			FirstSeen:       client.FirstSeen,
		}
	}

	return nil
}

func (sf *SpamFilter) periodicCleanup() {
	ticker := time.NewTicker(sf.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		sf.cleanup()
	}
}

func (sf *SpamFilter) cleanup() {
	now := time.Now()
	sf.mu.Lock()
	defer sf.mu.Unlock()

	// Clean up old client entries (older than 1 hour with no recent activity)
	for ip, client := range sf.clients {
		client.mu.RLock()
		lastActivity := client.LastRequestTime
		isInCooldown := now.Before(client.CooldownUntil)
		client.mu.RUnlock()

		// Keep clients that are in cooldown or had recent activity
		if !isInCooldown && now.Sub(lastActivity) > time.Hour {
			delete(sf.clients, ip)
		}
	}

	// Clean up expired blocks
	for ip, blockedUntil := range sf.blockedIPs {
		if now.After(blockedUntil) {
			delete(sf.blockedIPs, ip)
		}
	}
}

func NormalizeIP(ip string) string {
	// Parse the IP to handle IPv6 and ensure consistent format
	if parsedIP := net.ParseIP(ip); parsedIP != nil {
		return parsedIP.String()
	}
	return ip
}

// GetTierName returns a human-readable name for a tier
func GetTierName(tier RateLimitTier) string {
	switch tier {
	case TierNormal:
		return "normal"
	case TierWarning:
		return "warning"
	case TierStrictCooldown:
		return "strict_cooldown"
	case TierBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.Index(xff, ","); comma != -1 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}

// isBadIP checks if an IP is risky using external API
func isBadIP(ip string) bool {
	// Skip localhost and private IPs
	endpoint := fmt.Sprintf("https://api.iprisk.info/v1/%s", ip)
	resp, err := http.Get(endpoint)
	if err != nil {
		log.Printf("Failed to check IP risk: %v", err)
		return false
	}
	defer resp.Body.Close()

	var riskResponse models.IPRiskResponse
	if err := json.NewDecoder(resp.Body).Decode(&riskResponse); err != nil {
		log.Printf("Failed to decode IP risk response: %v", err)
		return false
	}

	return riskResponse.DataCenter || riskResponse.OpenProxy || riskResponse.VPN
}
