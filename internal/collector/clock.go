package collector

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/taniwha3/thugshells/internal/models"
)

const (
	// DefaultClockSkewWarnThresholdMs is the threshold (in milliseconds) above which we log a warning
	DefaultClockSkewWarnThresholdMs = 2000
)

// ClockSkewCollector detects clock skew by comparing local time to server Date header
type ClockSkewCollector struct {
	deviceID              string
	clockSkewURL          string // Separate URL for clock skew checks (not ingest URL)
	authToken             string // Bearer token for authentication
	warnThresholdMs       int64
	client                *http.Client
	lastSkewMs            int64 // Last measured skew for change detection
	lastWarningLoggedTime time.Time
}

// ClockSkewCollectorConfig configures the clock skew collector
type ClockSkewCollectorConfig struct {
	DeviceID        string
	ClockSkewURL    string // URL to check for clock skew (default: ingest URL if not specified)
	AuthToken       string // Bearer token for authentication (reuse from upload config)
	WarnThresholdMs int64  // Threshold in milliseconds to warn (default: 2000)
}

// NewClockSkewCollector creates a new clock skew detector
func NewClockSkewCollector(cfg ClockSkewCollectorConfig) *ClockSkewCollector {
	warnThreshold := cfg.WarnThresholdMs
	if warnThreshold <= 0 {
		warnThreshold = DefaultClockSkewWarnThresholdMs
	}

	return &ClockSkewCollector{
		deviceID:        cfg.DeviceID,
		clockSkewURL:    cfg.ClockSkewURL,
		authToken:       cfg.AuthToken,
		warnThresholdMs: warnThreshold,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the collector name
func (c *ClockSkewCollector) Name() string {
	return "clock"
}

// Collect detects clock skew by comparing local time to server Date header
func (c *ClockSkewCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	if c.clockSkewURL == "" {
		// No URL configured, skip clock skew detection
		return []*models.Metric{}, nil
	}

	// Create GET request to health endpoint
	// Per M2 spec: many endpoints don't support HEAD or return proxy time on HEAD
	// Use same auth as uploads per engineering review
	req, err := http.NewRequestWithContext(ctx, "GET", c.clockSkewURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create clock skew request: %w", err)
	}

	// Add authentication if token provided (reuse from upload config)
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	// Capture local time immediately before request
	localBefore := time.Now()

	// Make request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check clock skew: %w", err)
	}
	defer resp.Body.Close()

	// Capture local time immediately after request
	localAfter := time.Now()

	// Parse Date header (RFC1123 format: "Mon, 02 Jan 2006 15:04:05 GMT")
	dateHeader := resp.Header.Get("Date")
	if dateHeader == "" {
		return nil, fmt.Errorf("server response missing Date header")
	}

	serverTime, err := http.ParseTime(dateHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Date header %q: %w", dateHeader, err)
	}

	// Estimate round-trip time and adjust for network latency
	// Use midpoint of request: local time = (localBefore + localAfter) / 2
	roundTripTime := localAfter.Sub(localBefore)
	localMidpoint := localBefore.Add(roundTripTime / 2)

	// Calculate skew: positive = local is ahead, negative = local is behind
	skew := localMidpoint.Sub(serverTime)
	skewMs := skew.Milliseconds()

	// Warn if skew exceeds threshold (but only log once per hour to avoid spam)
	absSkewMs := skewMs
	if absSkewMs < 0 {
		absSkewMs = -absSkewMs
	}

	if absSkewMs > c.warnThresholdMs {
		// Only log warning if we haven't logged in the last hour (avoid spam)
		now := time.Now()
		if now.Sub(c.lastWarningLoggedTime) > time.Hour {
			// Emit warning (will migrate to structured logger in Day 3)
			direction := "ahead"
			if skewMs < 0 {
				direction = "behind"
			}
			log.Printf("WARNING: Clock skew detected: local clock is %dms %s of server %s (threshold: %dms)",
				absSkewMs, direction, c.clockSkewURL, c.warnThresholdMs)
			c.lastWarningLoggedTime = now
		}
	}

	// Store for change detection
	c.lastSkewMs = skewMs

	// Emit metric
	metric := models.NewMetric("time.skew_ms", float64(skewMs), c.deviceID)

	return []*models.Metric{metric}, nil
}
