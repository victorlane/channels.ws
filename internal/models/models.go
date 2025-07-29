package models

import (
	"time"
)

type Metadata struct {
	ID           int64     `json:"id" db:"id"`
	SessionID    int64     `json:"session_id" db:"session_id"`
	SubdomainID  string    `json:"subdomain_id" db:"subdomain_id"`
	MessageCount int       `json:"message_count" db:"message_count"`
	TotalPayload int64     `json:"total_payload" db:"total_payload"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

type Session struct {
	ID           int64      `json:"id" db:"id"`
	SubdomainID  string     `json:"subdomain_id" db:"subdomain_id"`
	ClientIP     string     `json:"client_ip" db:"client_ip"`
	StartTime    time.Time  `json:"start_time" db:"start_time"`
	EndTime      *time.Time `json:"end_time,omitempty" db:"end_time"`
	Duration     string     `json:"duration" db:"duration"`
	MessageCount int        `json:"message_count" db:"message_count"`
}

type Response struct {
	Messages  []string  `json:"messages"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"`
}

type IPRiskResponse struct {
	IpAddress  string `json:"ip_address"`
	DataCenter bool   `json:"data_center"`
	OpenProxy  bool   `json:"open_proxy"`
	VPN        bool   `json:"vpn"`
}
