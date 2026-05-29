package model

import "time"

type Route struct {
	ID            string
	Path          string
	UpstreamURL   string
	UpstreamID    *string
	Methods       []string
	Protocol      string // "http" | "grpc"
	MatchType     string // "exact" | "prefix"
	Enabled       bool
	AuthRequired  bool
	RateLimitRPM  int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Upstream struct {
	ID         string
	Name       string
	TargetURL  string
	Protocol   string
	HealthPath string
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
