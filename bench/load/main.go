// load is a small HTTP load generator: it requests the given URLs round-robin
// from -c workers for -d, then prints one JSON line of throughput and latency.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	concurrency := flag.Int("c", 32, "concurrent workers")
	duration := flag.Duration("d", 15*time.Second, "test duration")
	warmup := flag.Duration("w", 2*time.Second, "warm-up before measuring")
	label := flag.String("label", "", "label for the result line")
	flag.Parse()

	var urls []string
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		if u := strings.TrimSpace(sc.Text()); u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "no URLs on stdin")
		os.Exit(2)
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{MaxIdleConns: *concurrency, MaxIdleConnsPerHost: *concurrency},
	}
	hit := func(u string) (time.Duration, int64, bool) {
		start := time.Now()
		res, err := client.Get(u)
		if err != nil {
			return 0, 0, false
		}
		n, _ := io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return time.Since(start), n, res.StatusCode == http.StatusOK
	}

	var next atomic.Int64
	run := func(until time.Time, record bool) ([]time.Duration, int64, int64) {
		var mu sync.Mutex
		var all []time.Duration
		var bytes, errors atomic.Int64
		var wg sync.WaitGroup
		for range *concurrency {
			wg.Go(func() {
				var mine []time.Duration
				for time.Now().Before(until) {
					d, n, ok := hit(urls[next.Add(1)%int64(len(urls))])
					if !ok {
						errors.Add(1)
						continue
					}
					bytes.Add(n)
					if record {
						mine = append(mine, d)
					}
				}
				mu.Lock()
				all = append(all, mine...)
				mu.Unlock()
			})
		}
		wg.Wait()
		return all, bytes.Load(), errors.Load()
	}

	run(time.Now().Add(*warmup), false)
	start := time.Now()
	lat, bytes, errs := run(start.Add(*duration), true)
	elapsed := time.Since(start)
	slices.Sort(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return float64(lat[min(int(p*float64(len(lat))), len(lat)-1)].Microseconds()) / 1000
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"label": *label, "requests": len(lat), "errors": errs,
		"rps":   float64(len(lat)) / elapsed.Seconds(),
		"p50ms": pct(0.50), "p90ms": pct(0.90), "p99ms": pct(0.99), "maxms": pct(1),
		"MBps": float64(bytes) / elapsed.Seconds() / 1e6,
	})
}
