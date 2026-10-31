package gh

import (
	"sync"
	"time"
)

// Bucket is a token bucket: it holds up to Capacity tokens and gains one every Refill.
type Bucket struct {
	mu       sync.Mutex
	capacity float64
	tokens   float64
	refill   time.Duration
	last     time.Time
	now      func() time.Time
}

func NewBucket(capacity int, refill time.Duration) *Bucket {
	return &Bucket{capacity: float64(capacity), tokens: float64(capacity), refill: refill, now: time.Now, last: time.Now()}
}

// Take spends a token, reporting false if none is left.
func (b *Bucket) Take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	b.tokens = min(b.capacity, b.tokens+float64(now.Sub(b.last))/float64(b.refill))
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
