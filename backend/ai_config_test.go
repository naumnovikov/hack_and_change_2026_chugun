package main

import (
	"testing"
	"time"
)

func TestAIRequestTimeout(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", 180 * time.Second}, {"180s", 180 * time.Second}, {"5m", 5 * time.Minute},
		{"1s", time.Second}, {"10m", 10 * time.Minute}, {"bad", 180 * time.Second},
		{"0", 180 * time.Second}, {"-1s", 180 * time.Second}, {"500ms", 180 * time.Second},
		{"11m", 180 * time.Second},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("AI_REQUEST_TIMEOUT", tc.value)
			if got := aiRequestTimeout(); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
