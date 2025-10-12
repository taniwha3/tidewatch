package monitoring

import (
	"fmt"
	"net/http"
	"time"
)

// DetectClockSkew measures clock skew between local system and remote server
// by comparing the server's Date header with local time.
//
// Per M2 specification:
// - Uses GET to dedicated health endpoint (not HEAD to ingest URL)
// - Many ingest endpoints don't support HEAD or return proxy time
// - VictoriaMetrics: /health returns reliable Date headers
//
// Returns positive duration if local clock is ahead, negative if behind.
func DetectClockSkew(url, authToken string, timeout time.Duration) (time.Duration, error) {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Reuse auth token from config if provided
	// Per engineering review: clock skew check should use same auth as uploads
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	// Measure local time before and after request
	localBefore := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	localAfter := time.Now()

	// Parse Date header from server response
	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		return 0, fmt.Errorf("no Date header in response")
	}

	serverTime, err := http.ParseTime(dateStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse Date header %q: %w", dateStr, err)
	}

	// Estimate local time at moment of server response
	// Account for round-trip time by taking midpoint
	roundTrip := localAfter.Sub(localBefore)
	localEstimate := localBefore.Add(roundTrip / 2)

	// Calculate skew (positive = local ahead, negative = local behind)
	skew := localEstimate.Sub(serverTime)

	return skew, nil
}

// ClockSkewResult contains clock skew measurement details
type ClockSkewResult struct {
	Skew       time.Duration // Skew duration (positive = local ahead)
	SkewMs     int64         // Skew in milliseconds
	ServerTime time.Time     // Server time from Date header
	LocalTime  time.Time     // Local time estimate
	RoundTrip  time.Duration // HTTP round-trip time
}

// DetectClockSkewDetailed performs clock skew detection and returns detailed results
func DetectClockSkewDetailed(url, authToken string, timeout time.Duration) (*ClockSkewResult, error) {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Reuse auth token from config
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	localBefore := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	localAfter := time.Now()

	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		return nil, fmt.Errorf("no Date header in response")
	}

	serverTime, err := http.ParseTime(dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Date header: %w", err)
	}

	roundTrip := localAfter.Sub(localBefore)
	localEstimate := localBefore.Add(roundTrip / 2)
	skew := localEstimate.Sub(serverTime)

	return &ClockSkewResult{
		Skew:       skew,
		SkewMs:     skew.Milliseconds(),
		ServerTime: serverTime,
		LocalTime:  localEstimate,
		RoundTrip:  roundTrip,
	}, nil
}
