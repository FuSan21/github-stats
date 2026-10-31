// report turns run.sh's results.jsonl (on stdin) into markdown tables comparing bun and go.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type load struct {
	Requests int     `json:"requests"`
	Errors   int     `json:"errors"`
	RPS      float64 `json:"rps"`
	P50      float64 `json:"p50ms"`
	P90      float64 `json:"p90ms"`
	P99      float64 `json:"p99ms"`
	Max      float64 `json:"maxms"`
}

type row struct {
	Impl        string  `json:"impl"`
	Kind        string  `json:"kind"`
	Card        string  `json:"card"`
	Concurrency int     `json:"concurrency"`
	BuildS      float64 `json:"build_s"`
	ImageMB     float64 `json:"image_mb"`
	ReadyMS     float64 `json:"ready_ms"`
	FirstCardMS float64 `json:"first_card_ms"`
	IdleMiB     float64 `json:"idle_mib"`
	WarmIdleMiB float64 `json:"warm_idle_mib"`
	CPUMS       float64 `json:"cpu_ms"`
	WallMS      float64 `json:"wall_ms"`
	RSSMiB      float64 `json:"rss_mib"`
	PeakMiB     float64 `json:"peak_mib"`
	Load        load    `json:"load"`
}

// cpuPerReq is CPU milliseconds spent per successful request.
func (r row) cpuPerReq() float64 {
	if r.Load.Requests == 0 {
		return 0
	}
	return r.CPUMS / float64(r.Load.Requests)
}

// ratio is how many times better go is than bun; higherIsBetter flips it for throughput.
func ratio(bun, gov float64, higherIsBetter bool) string {
	if bun == 0 || gov == 0 {
		return "–"
	}
	r := bun / gov
	if higherIsBetter {
		r = gov / bun
	}
	return fmt.Sprintf("%.1f×", r)
}

func main() {
	byKey := map[string]row{}
	var cards []string
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var r row
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		key := r.Kind + "/" + r.Card + "/" + fmt.Sprint(r.Concurrency)
		if r.Kind == "card" && r.Impl == "bun" {
			cards = append(cards, r.Card)
		}
		byKey[r.Impl+"/"+key] = r
	}
	get := func(impl, kind, card string, c int) row { return byKey[impl+"/"+kind+"/"+card+"/"+fmt.Sprint(c)] }

	b, g := get("bun", "startup", "", 0), get("go", "startup", "", 0)
	bs, gs := get("bun", "settled", "", 0), get("go", "settled", "", 0)
	fmt.Println("| | Bun | Go | Go vs Bun |\n| --- | ---: | ---: | ---: |")
	line := func(name, unit string, bv, gv float64, higherBetter bool) {
		fmt.Printf("| %s | %.1f %s | %.1f %s | %s |\n", name, bv, unit, gv, unit, ratio(bv, gv, higherBetter))
	}
	line("Image size", "MB", b.ImageMB, g.ImageMB, false)
	line("Build time (no cache)", "s", b.BuildS, g.BuildS, false)
	line("Start to first `/healthz`", "ms", b.ReadyMS, g.ReadyMS, false)
	line("Memory at idle", "MiB", b.IdleMiB, g.IdleMiB, false)
	line("Memory at idle, data cached", "MiB", b.WarmIdleMiB, g.WarmIdleMiB, false)
	line("Peak memory under load", "MiB", bs.PeakMiB, gs.PeakMiB, false)
	line("Memory 10 s after load", "MiB", bs.RSSMiB, gs.RSSMiB, false)

	fmt.Print("\n**Per card**, 32 concurrent clients, data cached:\n\n")
	fmt.Println("| Card | Bun req/s | Go req/s | Go vs Bun | Bun p99 | Go p99 | Bun CPU/req | Go CPU/req |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, c := range cards {
		br, gr := get("bun", "card", c, 0), get("go", "card", c, 0)
		fmt.Printf("| `%s` | %.0f | %.0f | %s | %.1f ms | %.1f ms | %.2f ms | %.2f ms |\n",
			c, br.Load.RPS, gr.Load.RPS, ratio(br.Load.RPS, gr.Load.RPS, true), br.Load.P99, gr.Load.P99, br.cpuPerReq(), gr.cpuPerReq())
	}

	fmt.Print("\n**Mixed** (all cards round-robin):\n\n")
	fmt.Println("| Clients | | req/s | p50 | p90 | p99 | max | CPU/req | errors |")
	fmt.Println("| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, c := range []int{1, 64} {
		for _, impl := range []string{"bun", "go"} {
			r := get(impl, "mixed", "", c)
			fmt.Printf("| %d | %s | %.0f | %.2f ms | %.2f ms | %.2f ms | %.1f ms | %.2f ms | %d |\n",
				c, strings.ToUpper(impl[:1])+impl[1:], r.Load.RPS, r.Load.P50, r.Load.P90, r.Load.P99, r.Load.Max, r.cpuPerReq(), r.Load.Errors)
		}
	}
}
