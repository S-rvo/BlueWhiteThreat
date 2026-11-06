package db

import (
	"testing"
	"time"
)

func cleanAlertingKeys() {
	RedisClient.Del(ctx, "urls_to_crawl", "alerting:cooldown:test_condition")
}

func TestQueueCapacityRatio_ReflectsQueueSize(t *testing.T) {
	setupTestRedisClient(t)
	cleanAlertingKeys()
	defer cleanAlertingKeys()

	ratio, err := QueueCapacityRatio()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ratio != 0 {
		t.Fatalf("empty queue ratio = %v, want 0", ratio)
	}

	for i := 0; i < 500; i++ {
		RedisClient.SAdd(ctx, "urls_to_crawl", "http://alerting-test-"+string(rune('a'+i%26))+string(rune('0'+i/26))+".onion")
	}

	ratio, err = QueueCapacityRatio()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 500.0 / float64(maxQueueSize)
	if ratio != want {
		t.Errorf("ratio = %v, want %v", ratio, want)
	}
}

func TestTryMarkAlertSent_FirstCallSucceedsSecondIsCoolingDown(t *testing.T) {
	setupTestRedisClient(t)
	cleanAlertingKeys()
	defer cleanAlertingKeys()

	ok, err := TryMarkAlertSent("test_condition", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected first call to succeed (no prior cooldown)")
	}

	ok, err = TryMarkAlertSent("test_condition", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected second call to be suppressed by cooldown")
	}
}

func TestTryMarkAlertSent_DifferentConditionsAreIndependent(t *testing.T) {
	setupTestRedisClient(t)
	cleanAlertingKeys()
	defer func() {
		RedisClient.Del(ctx, "alerting:cooldown:condition_a", "alerting:cooldown:condition_b")
	}()

	ok, err := TryMarkAlertSent("condition_a", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected condition_a to succeed")
	}

	ok, err = TryMarkAlertSent("condition_b", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected condition_b to succeed independently of condition_a's cooldown")
	}
}

func TestTryMarkAlertSent_ExpiresAfterCooldown(t *testing.T) {
	setupTestRedisClient(t)
	cleanAlertingKeys()
	defer cleanAlertingKeys()

	ok, err := TryMarkAlertSent("test_condition", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected first call to succeed")
	}

	time.Sleep(700 * time.Millisecond)

	ok, err = TryMarkAlertSent("test_condition", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected call to succeed again once the cooldown TTL expired")
	}
}
