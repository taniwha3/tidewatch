package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the overall health status
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusError    Status = "error"
)

// ComponentStatus represents the health of a single component
type ComponentStatus struct {
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// HealthReport represents the complete health status of the system
type HealthReport struct {
	Status     Status                     `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]ComponentStatus `json:"components"`
	Uptime     float64                    `json:"uptime_seconds"` // Uptime in seconds (numeric)
}

// Checker is the main health monitoring service
type Checker struct {
	mu         sync.RWMutex
	components map[string]ComponentStatus
	startTime  time.Time
	thresholds Thresholds
}

// Thresholds defines health status thresholds
type Thresholds struct {
	// Upload timing thresholds (seconds)
	UploadOKInterval       int   `json:"upload_ok_interval"`        // 2x upload interval
	UploadDegradedInterval int   `json:"upload_degraded_interval"`  // 10x upload interval
	UploadErrorInterval    int   `json:"upload_error_interval"`     // >10min

	// Pending metrics thresholds
	PendingOKLimit       int64 `json:"pending_ok_limit"`       // <5000
	PendingDegradedLimit int64 `json:"pending_degraded_limit"` // 5000-10000
	PendingErrorLimit    int64 `json:"pending_error_limit"`    // >10000

	// Clock skew threshold (milliseconds)
	ClockSkewThresholdMs int64 `json:"clock_skew_threshold_ms"` // Default: 2000ms
}

// DefaultThresholds returns sensible default thresholds
// Use ThresholdsFromUploadInterval for production to derive from actual config
func DefaultThresholds() Thresholds {
	return Thresholds{
		UploadOKInterval:       60,    // 2x 30s default interval
		UploadDegradedInterval: 300,   // 10x 30s default interval
		UploadErrorInterval:    600,   // 10 minutes
		PendingOKLimit:         5000,
		PendingDegradedLimit:   10000,
		PendingErrorLimit:      10000,
		ClockSkewThresholdMs:   2000,  // 2 seconds default
	}
}

// ThresholdsFromUploadInterval calculates health thresholds based on the upload interval
// This ensures health status accurately reflects the configured upload timing
func ThresholdsFromUploadInterval(uploadInterval time.Duration) Thresholds {
	uploadIntervalSec := int(uploadInterval.Seconds())

	// Handle sub-second intervals by enforcing a minimum of 1 second
	// This prevents zero thresholds which would cause immediate degraded/error status
	if uploadIntervalSec < 1 {
		uploadIntervalSec = 1
	}

	return Thresholds{
		// OK: uploads within 2× interval
		UploadOKInterval:       uploadIntervalSec * 2,

		// Degraded: no upload between 2×-10× interval
		UploadDegradedInterval: uploadIntervalSec * 10,

		// Error: no upload >10min (fixed at 600s per M2 spec)
		// The 10-minute threshold is a hard requirement regardless of upload interval
		// to ensure timely escalation when combined with high pending count
		UploadErrorInterval:    600,

		// Pending count thresholds are independent of upload interval
		PendingOKLimit:         5000,
		PendingDegradedLimit:   10000,
		PendingErrorLimit:      10000,

		// Clock skew threshold (default 2000ms, can be overridden)
		ClockSkewThresholdMs:   2000,
	}
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// NewChecker creates a new health checker
func NewChecker(thresholds Thresholds) *Checker {
	return &Checker{
		components: make(map[string]ComponentStatus),
		startTime:  time.Now(),
		thresholds: thresholds,
	}
}

// UpdateComponent updates the status of a specific component
func (c *Checker) UpdateComponent(name string, status ComponentStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()

	status.Timestamp = time.Now()
	c.components[name] = status
}

// UpdateCollectorStatus updates the health status of a collector
func (c *Checker) UpdateCollectorStatus(collectorName string, err error, metricsCollected int) {
	status := ComponentStatus{
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"metrics_collected": metricsCollected,
		},
	}

	if err != nil {
		status.Status = StatusError
		status.Message = err.Error()
	} else {
		status.Status = StatusOK
		status.Message = "collecting metrics"
	}

	c.UpdateComponent("collector."+collectorName, status)
}

// UpdateUploaderStatus updates the health status of the uploader
func (c *Checker) UpdateUploaderStatus(lastUploadTime time.Time, lastUploadErr error, pendingCount int64) {
	status := ComponentStatus{
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"last_upload_time": lastUploadTime.Format(time.RFC3339),
			"pending_count":    pendingCount,
		},
	}

	timeSinceUpload := time.Since(lastUploadTime).Seconds()

	// Calculate status based on thresholds
	if lastUploadErr != nil {
		status.Status = StatusError
		status.Message = lastUploadErr.Error()
	} else if timeSinceUpload > float64(c.thresholds.UploadErrorInterval) && pendingCount > c.thresholds.PendingErrorLimit {
		status.Status = StatusError
		status.Message = "no successful upload in over 10 minutes and high pending count"
	} else if timeSinceUpload > float64(c.thresholds.UploadDegradedInterval) {
		status.Status = StatusDegraded
		status.Message = "no upload within 10× interval threshold"
	} else if timeSinceUpload > float64(c.thresholds.UploadOKInterval) {
		// Degraded when time exceeds 2× interval (but less than 10× handled above)
		status.Status = StatusDegraded
		status.Message = "no upload within 2× interval threshold"
	} else if pendingCount >= c.thresholds.PendingDegradedLimit {
		status.Status = StatusDegraded
		status.Message = "high pending metric count"
	} else if pendingCount >= c.thresholds.PendingOKLimit {
		status.Status = StatusDegraded
		status.Message = "elevated pending metric count"
	} else {
		status.Status = StatusOK
		status.Message = "uploading metrics"
	}

	status.Details["time_since_upload_seconds"] = int64(timeSinceUpload)

	c.UpdateComponent("uploader", status)
}

// UpdateStorageStatus updates the health status of storage
func (c *Checker) UpdateStorageStatus(dbSize int64, walSize int64, pendingCount int64) {
	status := ComponentStatus{
		Status:    StatusOK,
		Message:   "storage operational",
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"database_size_bytes": dbSize,
			"wal_size_bytes":     walSize,
			"pending_count":      pendingCount,
		},
	}

	// Mark degraded if WAL is too large
	if walSize > 64*1024*1024 { // 64 MB
		status.Status = StatusDegraded
		status.Message = "WAL size exceeds threshold"
	}

	c.UpdateComponent("storage", status)
}

// UpdateClockSkewStatus updates the health status of time synchronization
func (c *Checker) UpdateClockSkewStatus(skewMs int64, err error) {
	status := ComponentStatus{
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"skew_ms": skewMs,
		},
	}

	// Get threshold from config (default: 2000ms)
	threshold := c.thresholds.ClockSkewThresholdMs
	if threshold == 0 {
		threshold = 2000 // Fallback to default if not set
	}

	if err != nil {
		status.Status = StatusError
		status.Message = err.Error()
	} else if skewMs > threshold || skewMs < -threshold {
		status.Status = StatusDegraded
		status.Message = "clock skew exceeds threshold"
	} else {
		status.Status = StatusOK
		status.Message = "time synchronized"
	}

	c.UpdateComponent("time", status)
}

// GetReport generates a complete health report
func (c *Checker) GetReport() HealthReport {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Deep copy components
	components := make(map[string]ComponentStatus, len(c.components))
	for k, v := range c.components {
		components[k] = v
	}

	// Calculate overall status
	overallStatus := c.calculateOverallStatus(components)

	return HealthReport{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Components: components,
		Uptime:     time.Since(c.startTime).Seconds(),
	}
}

// calculateOverallStatus determines the overall system status from component statuses
func (c *Checker) calculateOverallStatus(components map[string]ComponentStatus) Status {
	if len(components) == 0 {
		return StatusOK
	}

	collectorErrorCount := 0
	collectorTotalCount := 0
	hasError := false
	hasDegraded := false

	for name, component := range components {
		// Count collector statuses separately
		if len(name) >= 10 && name[:10] == "collector." {
			collectorTotalCount++
			if component.Status == StatusError {
				collectorErrorCount++
			}
		}

		switch component.Status {
		case StatusError:
			hasError = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	// Error if all collectors are failing
	if collectorTotalCount > 0 && collectorErrorCount == collectorTotalCount {
		return StatusError
	}

	// Error if uploader or storage is in error state
	if uploaderStatus, ok := components["uploader"]; ok && uploaderStatus.Status == StatusError {
		return StatusError
	}
	if storageStatus, ok := components["storage"]; ok && storageStatus.Status == StatusError {
		return StatusError
	}

	// Degraded if any component is degraded or at least one collector has error
	if hasDegraded || collectorErrorCount > 0 {
		return StatusDegraded
	}

	// Error if any critical component has error (but not handled above)
	if hasError {
		return StatusError
	}

	return StatusOK
}

// HTTPHandler creates an HTTP handler for the health endpoint
func (c *Checker) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := c.GetReport()

		w.Header().Set("Content-Type", "application/json")

		// Set HTTP status code based on health status
		switch report.Status {
		case StatusOK:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // Still return 200 for degraded
		case StatusError:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(report)
	}
}

// LivenessHandler returns a simple liveness probe (always returns 200 if process is running)
func (c *Checker) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	}
}

// ReadinessHandler returns a readiness probe (200 only if status is OK)
func (c *Checker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := c.GetReport()

		w.Header().Set("Content-Type", "application/json")

		if report.Status == StatusOK {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ready",
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "not_ready",
				"message": "system is not in OK state",
				"current_status": string(report.Status),
			})
		}
	}
}

// StartHTTPServer starts the health check HTTP server
func (c *Checker) StartHTTPServer(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", c.HTTPHandler())
	mux.HandleFunc("/health/live", c.LivenessHandler())
	mux.HandleFunc("/health/ready", c.ReadinessHandler())

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Graceful shutdown handler
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}
