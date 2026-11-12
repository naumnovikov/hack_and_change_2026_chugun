package main

import (
	"log"
	"os"
	"time"
)

// Keep the upper bound below the frontend proxy's 610-second read timeout.
func aiRequestTimeout() time.Duration {
	const fallback = 180 * time.Second
	raw := os.Getenv("AI_REQUEST_TIMEOUT")
	if raw == "" {
		return fallback
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout < time.Second || timeout > 10*time.Minute {
		log.Print("WARN: AI_REQUEST_TIMEOUT must be between 1s and 10m; using 180s")
		return fallback
	}
	return timeout
}
