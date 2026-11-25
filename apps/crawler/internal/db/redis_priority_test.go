package db

import "testing"

func TestQueuePriorityDoesNotTreatAmbiguousTermAsStrongSignal(t *testing.T) {
	if got := queuePriority("http://example.onion/database"); got != 0 {
		t.Fatalf("generic database priority = %d", got)
	}
	if got := queuePriority("http://example.onion/database-breach"); got != 1 {
		t.Fatalf("corroborated database priority = %d", got)
	}
	if got := queuePriority("http://example.onion/access-broker"); got != 2 {
		t.Fatalf("access broker priority = %d", got)
	}
}

func TestSelectPriorityURLPrefersSignalWithoutDroppingFallback(t *testing.T) {
	plain := "http://hash-only.onion"
	strong := "http://example.onion/ransomware"
	if got := selectPriorityURL([]string{plain, strong}); got != strong {
		t.Fatalf("selected %q, want %q", got, strong)
	}
	if got := selectPriorityURL([]string{plain}); got != plain {
		t.Fatalf("single fallback selected %q", got)
	}
}
