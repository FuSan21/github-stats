package gh

import (
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	b := NewBucket(2, time.Minute)
	b.now, b.last = func() time.Time { return now }, now

	if !b.Take() || !b.Take() || b.Take() {
		t.Fatal("a full bucket of 2 should allow exactly 2 takes")
	}
	now = now.Add(30 * time.Second)
	if b.Take() {
		t.Error("half a refill period should not add a token")
	}
	now = now.Add(30 * time.Second)
	if !b.Take() || b.Take() {
		t.Error("one refill period should add exactly one token")
	}
	now = now.Add(time.Hour)
	if !b.Take() || !b.Take() || b.Take() {
		t.Error("refills should stop at capacity")
	}
}
