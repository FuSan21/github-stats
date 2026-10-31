package gh

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// DataTTL is how long fetched GitHub data is reused.
const DataTTL = 15 * time.Minute

type entry struct {
	at       time.Time
	value    any
	hasValue bool
	pending  chan struct{} // non-nil while a load is running; closed when done
	runValue any
	runErr   error
}

// Cache memoises loads by key. Concurrent callers share one load, and if a
// refresh fails the previous value keeps being served.
type Cache struct {
	mu      sync.Mutex
	entries map[string]*entry
	now     func() time.Time
}

func NewCache() *Cache { return &Cache{entries: map[string]*entry{}, now: time.Now} }

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*entry{}
}

// Cached returns the value under key if younger than ttl, else runs load.
func Cached[T any](c *Cache, key string, ttl time.Duration, load func() (T, error)) (T, error) {
	c.mu.Lock()
	e := c.entries[key]
	if e == nil {
		e = &entry{}
		c.entries[key] = e
	}
	if e.hasValue && c.now().Sub(e.at) < ttl {
		v := e.value.(T)
		c.mu.Unlock()
		return v, nil
	}
	if ch := e.pending; ch != nil {
		c.mu.Unlock()
		<-ch
		c.mu.Lock()
		v, err := e.runValue, e.runErr
		c.mu.Unlock()
		if err != nil {
			var zero T
			return zero, err
		}
		return v.(T), nil
	}
	ch := make(chan struct{})
	e.pending = ch
	c.mu.Unlock()

	// A panic must still release the callers waiting on this load.
	v, err := func() (v T, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("loading %s panicked: %v", key, r)
			}
		}()
		return load()
	}()

	c.mu.Lock()
	defer c.mu.Unlock()
	e.pending = nil
	if err == nil {
		e.at, e.value, e.hasValue = c.now(), v, true
		e.runValue, e.runErr = v, nil
	} else if e.hasValue {
		log.Printf("refresh of %s failed, serving stale data: %v", key, err)
		v, err = e.value.(T), nil
		e.runValue, e.runErr = v, nil
	} else {
		e.runValue, e.runErr = nil, err
	}
	close(ch)
	return v, err
}
