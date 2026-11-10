package crawler

import (
	"testing"
	"time"
)

func TestAdaptiveRevisit(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Result
		m    map[string]string
		want time.Duration
	}{
		{"new", Result{Status: 200, Hash: "a"}, nil, 24 * time.Hour},
		{"stable", Result{Status: 304}, map[string]string{"interval_seconds": "86400"}, 48 * time.Hour},
		{"changed", Result{Status: 200, Hash: "b"}, map[string]string{"hash": "a", "interval_seconds": "86400"}, 12 * time.Hour},
		{"floor", Result{Status: 200, Hash: "b"}, map[string]string{"hash": "a", "interval_seconds": "3600"}, time.Hour},
		{"ceiling", Result{Status: 304}, map[string]string{"interval_seconds": "2592000"}, 30 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := RevisitInterval(tc.r, tc.m); got != tc.want {
				t.Fatalf("%v != %v", got, tc.want)
			}
		})
	}
}
