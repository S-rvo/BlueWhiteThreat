package main

import (
	"context"
	"testing"
	"time"
)

func TestStoppedWorkersDoNotClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { startCrawler(ctx, 0); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers failed to stop")
	}
}
