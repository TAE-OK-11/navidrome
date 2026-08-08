package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go/http3"
)

type options struct {
	baseline    string
	candidate   string
	apiPath     string
	rangePath   string
	artworkPath string
	duration    time.Duration
	concurrency int
	insecure    bool
}

type result struct {
	Name                  string  `json:"name"`
	Requests              int64   `json:"requests"`
	Errors                int64   `json:"errors"`
	ErrorRate             float64 `json:"error_rate"`
	TooEarly              int64   `json:"http_425"`
	RangeFailures         int64   `json:"range_failures"`
	Bytes                 int64   `json:"bytes"`
	RequestsPerSecond     float64 `json:"requests_per_second"`
	MegabitsPerSecond     float64 `json:"megabits_per_second"`
	P50Milliseconds       float64 `json:"p50_ms"`
	P95Milliseconds       float64 `json:"p95_ms"`
	P99Milliseconds       float64 `json:"p99_ms"`
	P99JitterMilliseconds float64 `json:"p99_minus_p50_ms"`
	MaxMilliseconds       float64 `json:"max_ms"`
}

type sample struct {
	duration time.Duration
	bytes    int64
	status   int
	rangeOK  bool
	err      error
}

func main() {
	var opts options
	flag.StringVar(&opts.baseline, "baseline", "", "quic-go deployment base URL")
	flag.StringVar(&opts.candidate, "candidate", "", "tokio-quiche deployment base URL")
	flag.StringVar(&opts.apiPath, "api-path", "/ping", "small API path")
	flag.StringVar(&opts.rangePath, "range-path", "", "audio stream path used with a Range request")
	flag.StringVar(&opts.artworkPath, "artwork-path", "", "artwork path")
	flag.DurationVar(&opts.duration, "duration", 60*time.Second, "duration per provider")
	flag.IntVar(&opts.concurrency, "concurrency", 32, "concurrent HTTP/3 streams")
	flag.BoolVar(&opts.insecure, "insecure", false, "skip certificate verification")
	flag.Parse()
	if opts.baseline == "" || opts.candidate == "" || opts.duration <= 0 || opts.concurrency < 1 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	baseline := run(ctx, "quic-go", opts.baseline, opts)
	candidate := run(ctx, "tokio-quiche", opts.candidate, opts)
	report := struct {
		Baseline  result   `json:"baseline"`
		Candidate result   `json:"candidate"`
		Passed    bool     `json:"passed"`
		Failures  []string `json:"failures,omitempty"`
	}{Baseline: baseline, Candidate: candidate}
	report.Failures = regressionFailures(baseline, candidate)
	report.Passed = len(report.Failures) == 0
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !report.Passed {
		os.Exit(1)
	}
}

func run(ctx context.Context, name, baseURL string, opts options) result {
	transport := &http3.Transport{TLSClientConfig: &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: opts.insecure, //nolint:gosec // explicit benchmark option
	}}
	defer transport.Close()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Minute}
	ctx, cancel := context.WithTimeout(ctx, opts.duration)
	defer cancel()

	work := make(chan int)
	samples := make(chan sample, opts.concurrency*2)
	var workers sync.WaitGroup
	for range opts.concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for sequence := range work {
				samples <- request(ctx, client, baseURL, opts, sequence)
			}
		}()
	}
	go func() {
		defer close(work)
		for sequence := 0; ; sequence++ {
			select {
			case work <- sequence:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(samples)
	}()

	started := time.Now()
	var durations []time.Duration
	var total, failed, tooEarly, rangeFailures, bytesRead atomic.Int64
	for current := range samples {
		total.Add(1)
		if current.err != nil || current.status < 200 || current.status >= 400 {
			failed.Add(1)
		}
		if current.status == http.StatusTooEarly {
			tooEarly.Add(1)
		}
		if !current.rangeOK {
			rangeFailures.Add(1)
		}
		bytesRead.Add(current.bytes)
		durations = append(durations, current.duration)
	}
	elapsed := time.Since(started).Seconds()
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	output := result{
		Name: name, Requests: total.Load(), Errors: failed.Load(), TooEarly: tooEarly.Load(),
		RangeFailures: rangeFailures.Load(), Bytes: bytesRead.Load(),
		RequestsPerSecond: float64(total.Load()) / elapsed,
		MegabitsPerSecond: float64(bytesRead.Load()*8) / elapsed / 1_000_000,
		P50Milliseconds:   milliseconds(quantile(durations, 0.50)),
		P95Milliseconds:   milliseconds(quantile(durations, 0.95)),
		P99Milliseconds:   milliseconds(quantile(durations, 0.99)),
		MaxMilliseconds:   milliseconds(quantile(durations, 1.0)),
	}
	if output.Requests > 0 {
		output.ErrorRate = float64(output.Errors) / float64(output.Requests)
	}
	output.P99JitterMilliseconds = output.P99Milliseconds - output.P50Milliseconds
	return output
}

func request(ctx context.Context, client *http.Client, baseURL string, opts options, sequence int) sample {
	path, isRange := opts.apiPath, false
	switch sequence % 20 {
	case 0, 1, 2, 3, 4, 5, 6:
		if opts.rangePath != "" {
			path, isRange = opts.rangePath, true
		}
	case 7, 8, 9:
		if opts.artworkPath != "" {
			path = opts.artworkPath
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return sample{err: err, rangeOK: !isRange}
	}
	if value := os.Getenv("NAVIDROME_BENCH_AUTHORIZATION"); value != "" {
		request.Header.Set("Authorization", value)
	}
	if value := os.Getenv("NAVIDROME_BENCH_COOKIE"); value != "" {
		request.Header.Set("Cookie", value)
	}
	if isRange {
		request.Header.Set("Range", "bytes=0-262143")
	}
	started := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return sample{duration: time.Since(started), err: err, rangeOK: !isRange}
	}
	count, readErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	return sample{
		duration: time.Since(started), bytes: count, status: response.StatusCode,
		rangeOK: !isRange || response.StatusCode == http.StatusPartialContent,
		err:     errors.Join(readErr, closeErr),
	}
}

func quantile(values []time.Duration, q float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(q*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func milliseconds(value time.Duration) float64 { return float64(value) / float64(time.Millisecond) }

func regressionFailures(baseline, candidate result) []string {
	var failures []string
	if candidate.TooEarly != 0 {
		failures = append(failures, "candidate returned HTTP 425")
	}
	if candidate.RangeFailures != 0 {
		failures = append(failures, "candidate broke Range/206 semantics")
	}
	if candidate.ErrorRate > baseline.ErrorRate+0.001 {
		failures = append(failures, "candidate error rate exceeds baseline by more than 0.1 percentage point")
	}
	if baseline.P95Milliseconds > 0 && candidate.P95Milliseconds > baseline.P95Milliseconds*1.10 {
		failures = append(failures, "p95 latency regressed by more than 10%")
	}
	if baseline.P99Milliseconds > 0 && candidate.P99Milliseconds > baseline.P99Milliseconds*1.15 {
		failures = append(failures, "p99 latency regressed by more than 15%")
	}
	if baseline.RequestsPerSecond > 0 && candidate.RequestsPerSecond < baseline.RequestsPerSecond*0.90 {
		failures = append(failures, "request throughput regressed by more than 10%")
	}
	return failures
}
