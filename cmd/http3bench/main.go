package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"
)

type benchConfig struct {
	baselineURL   string
	candidateURL  string
	apiPath       string
	rangePath     string
	artworkPath   string
	duration      time.Duration
	concurrency   int
	authorization string
	insecureTLS   bool
}

type benchResult struct {
	name         string
	requests     int64
	errors       int64
	tooEarly     int64
	rangeErrors  int64
	latencies    []time.Duration
}

func main() {
	cfg := benchConfig{}
	flag.StringVar(&cfg.baselineURL, "baseline", "", "baseline HTTPS endpoint (H1/H2 or legacy H3)")
	flag.StringVar(&cfg.candidateURL, "candidate", "", "candidate HTTPS endpoint (tokio-quiche H3)")
	flag.StringVar(&cfg.apiPath, "api-path", "/rest/ping.view?f=json", "lightweight API path")
	flag.StringVar(&cfg.rangePath, "range-path", "", "optional Range/206 validation path")
	flag.StringVar(&cfg.artworkPath, "artwork-path", "", "optional artwork path")
	flag.DurationVar(&cfg.duration, "duration", 2*time.Minute, "benchmark duration per target")
	flag.IntVar(&cfg.concurrency, "concurrency", 64, "concurrent workers per target")
	flag.BoolVar(&cfg.insecureTLS, "insecure", false, "skip TLS certificate verification")
	flag.Parse()

	if cfg.authorization == "" {
		cfg.authorization = strings.TrimSpace(os.Getenv("NAVIDROME_BENCH_AUTHORIZATION"))
	}
	if cfg.baselineURL == "" || cfg.candidateURL == "" {
		fmt.Fprintln(os.Stderr, "baseline and candidate URLs are required")
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	baseline := runBench(ctx, "baseline", cfg.baselineURL, cfg, false)
	candidate := runBench(ctx, "candidate", cfg.candidateURL, cfg, true)

	if err := compareResults(baseline, candidate); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark regression: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("baseline:  requests=%d errors=%d p95=%s p99=%s rps=%.1f\n",
		baseline.requests, baseline.errors, percentile(baseline.latencies, 0.95), percentile(baseline.latencies, 0.99), rps(baseline))
	fmt.Printf("candidate: requests=%d errors=%d too_early=%d range_errors=%d p95=%s p99=%s rps=%.1f\n",
		candidate.requests, candidate.errors, candidate.tooEarly, candidate.rangeErrors,
		percentile(candidate.latencies, 0.95), percentile(candidate.latencies, 0.99), rps(candidate))
}

func runBench(ctx context.Context, name, base string, cfg benchConfig, validateH3 bool) benchResult {
	client := newBenchClient(base, cfg.insecureTLS)
	defer func() { _ = client.Close() }()

	paths := []string{cfg.apiPath}
	if cfg.rangePath != "" {
		paths = append(paths, cfg.rangePath)
	}
	if cfg.artworkPath != "" {
		paths = append(paths, cfg.artworkPath)
	}

	var result benchResult
	result.name = name
	var latencyMu sync.Mutex
	stopAt := time.Now().Add(cfg.duration)
	var wg sync.WaitGroup
	for worker := 0; worker < cfg.concurrency; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			path := paths[workerID%len(paths)]
			for time.Now().Before(stopAt) {
				if ctx.Err() != nil {
					return
				}
				start := time.Now()
				status, err := doRequest(client, joinURL(base, path), cfg.authorization, strings.Contains(path, "stream"))
				latency := time.Since(start)
				latencyMu.Lock()
				result.latencies = append(result.latencies, latency)
				latencyMu.Unlock()
				atomic.AddInt64(&result.requests, 1)
				if err != nil || status >= 500 {
					atomic.AddInt64(&result.errors, 1)
					continue
				}
				if validateH3 && status == http.StatusTooEarly {
					atomic.AddInt64(&result.tooEarly, 1)
				}
				if strings.Contains(path, "stream") && status != http.StatusOK && status != http.StatusPartialContent {
					atomic.AddInt64(&result.rangeErrors, 1)
				}
			}
		}(worker)
	}
	wg.Wait()
	return result
}

func doRequest(client benchHTTPClient, url, authorization string, rangeRequest bool) (int, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if rangeRequest {
		req.Header.Set("Range", "bytes=0-65535")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

type benchHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
	Close() error
}

func newBenchClient(base string, insecure bool) benchHTTPClient {
	tlsConfig := &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // rollout comparison tool
	if strings.HasPrefix(strings.ToLower(base), "https://") {
		transport := &http3.Transport{TLSClientConfig: tlsConfig}
		return &http3BenchClient{
			Client: &http.Client{Transport: transport, Timeout: 60 * time.Second},
			closer: transport,
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http3BenchClient{Client: &http.Client{Transport: transport, Timeout: 60 * time.Second}}
}

type http3BenchClient struct {
	*http.Client
	closer io.Closer
}

func (c *http3BenchClient) Close() error {
	if c.closer != nil {
		return c.closer.Close()
	}
	return nil
}

func compareResults(baseline, candidate benchResult) error {
	if candidate.tooEarly > 0 {
		return fmt.Errorf("candidate returned %d HTTP 425 responses", candidate.tooEarly)
	}
	if candidate.rangeErrors > 0 {
		return fmt.Errorf("candidate returned %d stream/range failures", candidate.rangeErrors)
	}

	baseErrRate := errorRate(baseline)
	candErrRate := errorRate(candidate)
	if candErrRate-baseErrRate > 0.001 {
		return fmt.Errorf("candidate error rate %.4f exceeds baseline %.4f by >0.1pp", candErrRate, baseErrRate)
	}

	baseP95 := percentile(baseline.latencies, 0.95)
	candP95 := percentile(candidate.latencies, 0.95)
	if baseP95 > 0 && float64(candP95)/float64(baseP95) > 1.10 {
		return fmt.Errorf("candidate p95 %s is >10%% slower than baseline %s", candP95, baseP95)
	}

	baseP99 := percentile(baseline.latencies, 0.99)
	candP99 := percentile(candidate.latencies, 0.99)
	if baseP99 > 0 && float64(candP99)/float64(baseP99) > 1.15 {
		return fmt.Errorf("candidate p99 %s is >15%% slower than baseline %s", candP99, baseP99)
	}

	baseRPS := rps(baseline)
	candRPS := rps(candidate)
	if baseRPS > 0 && candRPS/baseRPS < 0.90 {
		return fmt.Errorf("candidate throughput %.1f rps is >10%% below baseline %.1f rps", candRPS, baseRPS)
	}
	return nil
}

func errorRate(result benchResult) float64 {
	if result.requests == 0 {
		return 0
	}
	return float64(result.errors) / float64(result.requests)
}

func rps(result benchResult) float64 {
	if len(result.latencies) == 0 {
		return 0
	}
	total := time.Duration(0)
	for _, latency := range result.latencies {
		total += latency
	}
	avg := total / time.Duration(len(result.latencies))
	if avg == 0 {
		return 0
	}
	return float64(time.Second) / float64(avg)
}

func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

func joinURL(base, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
