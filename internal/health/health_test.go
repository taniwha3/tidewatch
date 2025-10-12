package health

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewChecker(t *testing.T) {
	thresholds := DefaultThresholds()
	checker := NewChecker(thresholds)

	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}

	if len(checker.components) != 0 {
		t.Errorf("Expected 0 components, got %d", len(checker.components))
	}

	if checker.thresholds != thresholds {
		t.Error("Thresholds not set correctly")
	}
}

func TestUpdateComponent(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	status := ComponentStatus{
		Status:  StatusOK,
		Message: "test message",
		Details: map[string]interface{}{
			"key": "value",
		},
	}

	checker.UpdateComponent("test-component", status)

	report := checker.GetReport()
	component, exists := report.Components["test-component"]

	if !exists {
		t.Fatal("Component not found in report")
	}

	if component.Status != StatusOK {
		t.Errorf("Expected status OK, got %s", component.Status)
	}

	if component.Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", component.Message)
	}

	if component.Details["key"] != "value" {
		t.Errorf("Expected detail key='value', got %v", component.Details["key"])
	}

	if component.Timestamp.IsZero() {
		t.Error("Timestamp not set")
	}
}

func TestUpdateCollectorStatus(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	tests := []struct {
		name             string
		collectorName    string
		err              error
		metricsCollected int
		expectedStatus   Status
		expectedMessage  string
	}{
		{
			name:             "successful collection",
			collectorName:    "cpu",
			err:              nil,
			metricsCollected: 10,
			expectedStatus:   StatusOK,
			expectedMessage:  "collecting metrics",
		},
		{
			name:             "failed collection",
			collectorName:    "memory",
			err:              errors.New("failed to read /proc/meminfo"),
			metricsCollected: 0,
			expectedStatus:   StatusError,
			expectedMessage:  "failed to read /proc/meminfo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.UpdateCollectorStatus(tt.collectorName, tt.err, tt.metricsCollected)

			report := checker.GetReport()
			componentName := "collector." + tt.collectorName
			component, exists := report.Components[componentName]

			if !exists {
				t.Fatalf("Component %s not found", componentName)
			}

			if component.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, component.Status)
			}

			if component.Message != tt.expectedMessage {
				t.Errorf("Expected message '%s', got '%s'", tt.expectedMessage, component.Message)
			}

			if component.Details["metrics_collected"] != tt.metricsCollected {
				t.Errorf("Expected metrics_collected=%d, got %v", tt.metricsCollected, component.Details["metrics_collected"])
			}
		})
	}
}

func TestUpdateUploaderStatus(t *testing.T) {
	thresholds := DefaultThresholds()
	checker := NewChecker(thresholds)

	now := time.Now()

	tests := []struct {
		name           string
		lastUploadTime time.Time
		lastUploadErr  error
		pendingCount   int64
		expectedStatus Status
	}{
		{
			name:           "ok - recent upload, low pending",
			lastUploadTime: now.Add(-10 * time.Second),
			lastUploadErr:  nil,
			pendingCount:   100,
			expectedStatus: StatusOK,
		},
		{
			name:           "degraded - elevated pending count",
			lastUploadTime: now.Add(-10 * time.Second),
			lastUploadErr:  nil,
			pendingCount:   5500,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "degraded - high pending count",
			lastUploadTime: now.Add(-10 * time.Second),
			lastUploadErr:  nil,
			pendingCount:   9000,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "degraded - no upload within degraded threshold",
			lastUploadTime: now.Add(-time.Duration(thresholds.UploadDegradedInterval+10) * time.Second),
			lastUploadErr:  nil,
			pendingCount:   100,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "error - upload error",
			lastUploadTime: now.Add(-10 * time.Second),
			lastUploadErr:  errors.New("upload failed"),
			pendingCount:   100,
			expectedStatus: StatusError,
		},
		{
			name:           "error - no upload and high pending",
			lastUploadTime: now.Add(-time.Duration(thresholds.UploadErrorInterval+10) * time.Second),
			lastUploadErr:  nil,
			pendingCount:   15000,
			expectedStatus: StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.UpdateUploaderStatus(tt.lastUploadTime, tt.lastUploadErr, tt.pendingCount)

			report := checker.GetReport()
			component, exists := report.Components["uploader"]

			if !exists {
				t.Fatal("Uploader component not found")
			}

			if component.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s (message: %s)", tt.expectedStatus, component.Status, component.Message)
			}

			if component.Details["pending_count"] != tt.pendingCount {
				t.Errorf("Expected pending_count=%d, got %v", tt.pendingCount, component.Details["pending_count"])
			}
		})
	}
}

func TestUpdateStorageStatus(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	tests := []struct {
		name           string
		dbSize         int64
		walSize        int64
		pendingCount   int64
		expectedStatus Status
	}{
		{
			name:           "ok - normal wal size",
			dbSize:         10 * 1024 * 1024,
			walSize:        10 * 1024 * 1024,
			pendingCount:   100,
			expectedStatus: StatusOK,
		},
		{
			name:           "degraded - wal exceeds threshold",
			dbSize:         10 * 1024 * 1024,
			walSize:        65 * 1024 * 1024,
			pendingCount:   100,
			expectedStatus: StatusDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.UpdateStorageStatus(tt.dbSize, tt.walSize, tt.pendingCount)

			report := checker.GetReport()
			component, exists := report.Components["storage"]

			if !exists {
				t.Fatal("Storage component not found")
			}

			if component.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, component.Status)
			}

			if component.Details["database_size_bytes"] != tt.dbSize {
				t.Errorf("Expected database_size_bytes=%d, got %v", tt.dbSize, component.Details["database_size_bytes"])
			}

			if component.Details["wal_size_bytes"] != tt.walSize {
				t.Errorf("Expected wal_size_bytes=%d, got %v", tt.walSize, component.Details["wal_size_bytes"])
			}
		})
	}
}

func TestUpdateClockSkewStatus(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	tests := []struct {
		name           string
		skewMs         int64
		err            error
		expectedStatus Status
	}{
		{
			name:           "ok - minimal skew",
			skewMs:         100,
			err:            nil,
			expectedStatus: StatusOK,
		},
		{
			name:           "ok - 1 second skew",
			skewMs:         1000,
			err:            nil,
			expectedStatus: StatusOK,
		},
		{
			name:           "degraded - skew exceeds threshold positive",
			skewMs:         2500,
			err:            nil,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "degraded - skew exceeds threshold negative",
			skewMs:         -3000,
			err:            nil,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "error - clock skew check failed",
			skewMs:         0,
			err:            errors.New("connection timeout"),
			expectedStatus: StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.UpdateClockSkewStatus(tt.skewMs, tt.err)

			report := checker.GetReport()
			component, exists := report.Components["time"]

			if !exists {
				t.Fatal("Time component not found")
			}

			if component.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, component.Status)
			}

			if component.Details["skew_ms"] != tt.skewMs {
				t.Errorf("Expected skew_ms=%d, got %v", tt.skewMs, component.Details["skew_ms"])
			}
		})
	}
}

func TestCalculateOverallStatus(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(*Checker)
		expectedStatus Status
	}{
		{
			name: "ok - all components ok",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateCollectorStatus("memory", nil, 5)
				c.UpdateUploaderStatus(time.Now(), nil, 100)
				c.UpdateStorageStatus(1024, 1024, 100)
			},
			expectedStatus: StatusOK,
		},
		{
			name: "ok - no components",
			setupFunc: func(c *Checker) {
				// No components
			},
			expectedStatus: StatusOK,
		},
		{
			name: "degraded - one collector error",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", nil, 5)
				c.UpdateUploaderStatus(time.Now(), nil, 100)
			},
			expectedStatus: StatusDegraded,
		},
		{
			name: "degraded - uploader degraded",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateUploaderStatus(time.Now(), nil, 6000) // High pending
			},
			expectedStatus: StatusDegraded,
		},
		{
			name: "degraded - time component degraded",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateUploaderStatus(time.Now(), nil, 100)
				c.UpdateClockSkewStatus(3000, nil) // High skew
			},
			expectedStatus: StatusDegraded,
		},
		{
			name: "error - all collectors failed",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", errors.New("failed"), 0)
				c.UpdateCollectorStatus("disk", errors.New("failed"), 0)
			},
			expectedStatus: StatusError,
		},
		{
			name: "error - uploader error",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateUploaderStatus(time.Now(), errors.New("upload failed"), 100)
			},
			expectedStatus: StatusError,
		},
		{
			name: "error - storage error (simulated with component update)",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateComponent("storage", ComponentStatus{
					Status:  StatusError,
					Message: "disk full",
				})
			},
			expectedStatus: StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(DefaultThresholds())
			tt.setupFunc(checker)

			report := checker.GetReport()

			if report.Status != tt.expectedStatus {
				t.Errorf("Expected overall status %s, got %s", tt.expectedStatus, report.Status)
				t.Logf("Components: %+v", report.Components)
			}
		})
	}
}

func TestGetReport(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	checker.UpdateCollectorStatus("cpu", nil, 10)
	checker.UpdateUploaderStatus(time.Now(), nil, 100)

	report := checker.GetReport()

	if report.Status == "" {
		t.Error("Report status is empty")
	}

	if report.Timestamp.IsZero() {
		t.Error("Report timestamp is zero")
	}

	if report.Uptime <= 0 {
		t.Error("Report uptime should be positive")
	}

	// Verify uptime is numeric seconds (not a duration string)
	if report.Uptime > 3600 {
		t.Errorf("Report uptime should be in seconds, got %f (too large)", report.Uptime)
	}

	if len(report.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(report.Components))
	}

	if _, exists := report.Components["collector.cpu"]; !exists {
		t.Error("CPU collector component not found")
	}

	if _, exists := report.Components["uploader"]; !exists {
		t.Error("Uploader component not found")
	}
}

func TestHTTPHandler(t *testing.T) {
	tests := []struct {
		name               string
		setupFunc          func(*Checker)
		expectedStatusCode int
		expectedStatus     Status
	}{
		{
			name: "ok status returns 200",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateUploaderStatus(time.Now(), nil, 100)
			},
			expectedStatusCode: http.StatusOK,
			expectedStatus:     StatusOK,
		},
		{
			name: "degraded status returns 200",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", nil, 5)
			},
			expectedStatusCode: http.StatusOK,
			expectedStatus:     StatusDegraded,
		},
		{
			name: "error status returns 503",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", errors.New("failed"), 0)
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedStatus:     StatusError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(DefaultThresholds())
			tt.setupFunc(checker)

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()

			handler := checker.HTTPHandler()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			if resp.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
			}

			var report HealthReport
			if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if report.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, report.Status)
			}
		})
	}
}

func TestLivenessHandler(t *testing.T) {
	// Liveness should always return 200, regardless of health status
	tests := []struct {
		name      string
		setupFunc func(*Checker)
	}{
		{
			name: "healthy system",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
			},
		},
		{
			name: "unhealthy system",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", errors.New("failed"), 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(DefaultThresholds())
			tt.setupFunc(checker)

			req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
			w := httptest.NewRecorder()

			handler := checker.LivenessHandler()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status code 200, got %d", resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			if !contains(string(body), "alive") {
				t.Error("Response should contain 'alive'")
			}
		})
	}
}

func TestReadinessHandler(t *testing.T) {
	tests := []struct {
		name               string
		setupFunc          func(*Checker)
		expectedStatusCode int
	}{
		{
			name: "ready - ok status",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", nil, 10)
				c.UpdateUploaderStatus(time.Now(), nil, 100)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "not ready - degraded status",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", nil, 5)
			},
			expectedStatusCode: http.StatusServiceUnavailable,
		},
		{
			name: "not ready - error status",
			setupFunc: func(c *Checker) {
				c.UpdateCollectorStatus("cpu", errors.New("failed"), 0)
				c.UpdateCollectorStatus("memory", errors.New("failed"), 0)
			},
			expectedStatusCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(DefaultThresholds())
			tt.setupFunc(checker)

			req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
			w := httptest.NewRecorder()

			handler := checker.ReadinessHandler()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}

			var response map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.expectedStatusCode == http.StatusOK {
				if response["status"] != "ready" {
					t.Errorf("Expected status 'ready', got %v", response["status"])
				}
			} else {
				if response["status"] != "not_ready" {
					t.Errorf("Expected status 'not_ready', got %v", response["status"])
				}
			}
		})
	}
}

func TestStartHTTPServer(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		err := checker.StartHTTPServer(ctx, ":19100")
		errChan <- err
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get("http://localhost:19100/health")
	if err != nil {
		t.Fatalf("Failed to connect to health server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Test liveness endpoint
	resp, err = http.Get("http://localhost:19100/health/live")
	if err != nil {
		t.Fatalf("Failed to connect to liveness endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected liveness status 200, got %d", resp.StatusCode)
	}

	// Test readiness endpoint
	resp, err = http.Get("http://localhost:19100/health/ready")
	if err != nil {
		t.Fatalf("Failed to connect to readiness endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Should be OK since no components are set
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected readiness status 200, got %d", resp.StatusCode)
	}

	// Cancel context to stop server
	cancel()

	// Wait for server to stop
	select {
	case err := <-errChan:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}
}

func TestConcurrentAccess(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	// Test concurrent reads and writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				checker.UpdateCollectorStatus("cpu", nil, id*10)
				_ = checker.GetReport()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify no race conditions by getting final report
	report := checker.GetReport()
	if report.Status == "" {
		t.Error("Report status is empty after concurrent access")
	}
}

func TestThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	if thresholds.UploadOKInterval <= 0 {
		t.Error("UploadOKInterval should be positive")
	}

	if thresholds.UploadDegradedInterval <= thresholds.UploadOKInterval {
		t.Error("UploadDegradedInterval should be greater than UploadOKInterval")
	}

	if thresholds.PendingOKLimit <= 0 {
		t.Error("PendingOKLimit should be positive")
	}

	if thresholds.PendingDegradedLimit <= thresholds.PendingOKLimit {
		t.Error("PendingDegradedLimit should be greater than PendingOKLimit")
	}
}

func TestThresholdsFromUploadInterval(t *testing.T) {
	tests := []struct {
		name                   string
		uploadInterval         time.Duration
		expectedOK             int
		expectedDegraded       int
		expectedError          int
		expectedPendingOK      int64
		expectedPendingDegraded int64
	}{
		{
			name:                   "30s default interval",
			uploadInterval:         30 * time.Second,
			expectedOK:             60,   // 2× 30s
			expectedDegraded:       300,  // 10× 30s
			expectedError:          600,  // Always 10 minutes per M2 spec
			expectedPendingOK:      5000,
			expectedPendingDegraded: 10000,
		},
		{
			name:                   "5m interval",
			uploadInterval:         5 * time.Minute,
			expectedOK:             600,  // 2× 5m = 10m
			expectedDegraded:       3000, // 10× 5m = 50m
			expectedError:          600,  // Always 10 minutes per M2 spec
			expectedPendingOK:      5000,
			expectedPendingDegraded: 10000,
		},
		{
			name:                   "1m interval",
			uploadInterval:         1 * time.Minute,
			expectedOK:             120, // 2× 1m = 2m
			expectedDegraded:       600, // 10× 1m = 10m
			expectedError:          600, // Always 10 minutes per M2 spec
			expectedPendingOK:      5000,
			expectedPendingDegraded: 10000,
		},
		{
			name:                   "10s interval",
			uploadInterval:         10 * time.Second,
			expectedOK:             20,  // 2× 10s
			expectedDegraded:       100, // 10× 10s
			expectedError:          600, // Always 10 minutes per M2 spec
			expectedPendingOK:      5000,
			expectedPendingDegraded: 10000,
		},
		{
			name:                   "2m interval",
			uploadInterval:         2 * time.Minute,
			expectedOK:             240,  // 2× 2m = 4m
			expectedDegraded:       1200, // 10× 2m = 20m
			expectedError:          600,  // Always 10 minutes per M2 spec
			expectedPendingOK:      5000,
			expectedPendingDegraded: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thresholds := ThresholdsFromUploadInterval(tt.uploadInterval)

			if thresholds.UploadOKInterval != tt.expectedOK {
				t.Errorf("UploadOKInterval: expected %d, got %d", tt.expectedOK, thresholds.UploadOKInterval)
			}

			if thresholds.UploadDegradedInterval != tt.expectedDegraded {
				t.Errorf("UploadDegradedInterval: expected %d, got %d", tt.expectedDegraded, thresholds.UploadDegradedInterval)
			}

			if thresholds.UploadErrorInterval != tt.expectedError {
				t.Errorf("UploadErrorInterval: expected %d, got %d", tt.expectedError, thresholds.UploadErrorInterval)
			}

			if thresholds.PendingOKLimit != tt.expectedPendingOK {
				t.Errorf("PendingOKLimit: expected %d, got %d", tt.expectedPendingOK, thresholds.PendingOKLimit)
			}

			if thresholds.PendingDegradedLimit != tt.expectedPendingDegraded {
				t.Errorf("PendingDegradedLimit: expected %d, got %d", tt.expectedPendingDegraded, thresholds.PendingDegradedLimit)
			}

			// Verify relationships
			if thresholds.UploadDegradedInterval <= thresholds.UploadOKInterval {
				t.Error("UploadDegradedInterval should be greater than UploadOKInterval")
			}

			// Error threshold must always be exactly 10 minutes per M2 spec
			if thresholds.UploadErrorInterval != 600 {
				t.Errorf("UploadErrorInterval must be exactly 600s (10 minutes) per M2 spec, got %d", thresholds.UploadErrorInterval)
			}
		})
	}
}

func TestThresholdsFromUploadIntervalWithRealWorldScenarios(t *testing.T) {
	// Test that a 5-minute upload interval doesn't trigger degraded at 5 minutes
	uploadInterval := 5 * time.Minute
	thresholds := ThresholdsFromUploadInterval(uploadInterval)
	checker := NewChecker(thresholds)

	now := time.Now()

	// Scenario: Last upload was 6 minutes ago with 5-minute interval
	// Should be OK because 6m < 10m (2× interval)
	lastUploadTime := now.Add(-6 * time.Minute)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 100)

	report := checker.GetReport()
	uploaderStatus := report.Components["uploader"]

	if uploaderStatus.Status != StatusOK {
		t.Errorf("With 5m interval, 6m since upload should be OK, got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}

	// Scenario: Last upload was 35 minutes ago with 5-minute interval
	// Should be DEGRADED because 35m > 10m (2× interval) but < 50m (10× interval)
	lastUploadTime = now.Add(-35 * time.Minute)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 100)

	report = checker.GetReport()
	uploaderStatus = report.Components["uploader"]

	if uploaderStatus.Status != StatusDegraded {
		t.Errorf("With 5m interval, 35m since upload should be DEGRADED, got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}

	// Scenario: Last upload was 11 minutes ago with high pending
	// Should be ERROR because > 10m (error threshold) with high pending (>10000)
	// This verifies the 10-minute error threshold works regardless of upload interval
	lastUploadTime = now.Add(-11 * time.Minute)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 15000)

	report = checker.GetReport()
	uploaderStatus = report.Components["uploader"]

	if uploaderStatus.Status != StatusError {
		t.Errorf("With 5m interval, 11m since upload with high pending should be ERROR (10min threshold), got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}

	// Scenario: Last upload was 51 minutes ago with high pending
	// Should also be ERROR (well beyond the 10-minute threshold)
	lastUploadTime = now.Add(-51 * time.Minute)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 15000)

	report = checker.GetReport()
	uploaderStatus = report.Components["uploader"]

	if uploaderStatus.Status != StatusError {
		t.Errorf("With 5m interval, 51m since upload with high pending should be ERROR, got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}
}

func TestJSONSerialization(t *testing.T) {
	checker := NewChecker(DefaultThresholds())

	checker.UpdateCollectorStatus("cpu", nil, 10)
	checker.UpdateUploaderStatus(time.Now(), nil, 100)

	// Sleep briefly to ensure non-zero uptime
	time.Sleep(10 * time.Millisecond)

	report := checker.GetReport()

	// Marshal to JSON
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal report: %v", err)
	}

	// Verify uptime is serialized as numeric seconds, not a duration string
	dataStr := string(data)
	if contains(dataStr, "ms") || contains(dataStr, "µs") || contains(dataStr, "ns") {
		t.Errorf("Uptime appears to be serialized as duration string: %s", dataStr)
	}

	// Unmarshal from JSON
	var decoded HealthReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	if decoded.Status != report.Status {
		t.Errorf("Status mismatch: %s != %s", decoded.Status, report.Status)
	}

	if len(decoded.Components) != len(report.Components) {
		t.Errorf("Components count mismatch: %d != %d", len(decoded.Components), len(report.Components))
	}

	// Verify uptime is a valid numeric seconds value
	if decoded.Uptime <= 0 {
		t.Errorf("Decoded uptime should be positive, got %f", decoded.Uptime)
	}

	if decoded.Uptime > 3600 {
		t.Errorf("Decoded uptime should be < 1 hour for this test, got %f", decoded.Uptime)
	}
}

func TestSubSecondUploadInterval(t *testing.T) {
	tests := []struct {
		name             string
		uploadInterval   time.Duration
		expectedOK       int
		expectedDegraded int
	}{
		{
			name:             "500ms interval",
			uploadInterval:   500 * time.Millisecond,
			expectedOK:       2,  // min 1s * 2
			expectedDegraded: 10, // min 1s * 10
		},
		{
			name:             "100ms interval",
			uploadInterval:   100 * time.Millisecond,
			expectedOK:       2,
			expectedDegraded: 10,
		},
		{
			name:             "1ns interval",
			uploadInterval:   1 * time.Nanosecond,
			expectedOK:       2,
			expectedDegraded: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thresholds := ThresholdsFromUploadInterval(tt.uploadInterval)

			if thresholds.UploadOKInterval != tt.expectedOK {
				t.Errorf("UploadOKInterval: expected %d, got %d", tt.expectedOK, thresholds.UploadOKInterval)
			}

			if thresholds.UploadDegradedInterval != tt.expectedDegraded {
				t.Errorf("UploadDegradedInterval: expected %d, got %d", tt.expectedDegraded, thresholds.UploadDegradedInterval)
			}

			// Error threshold should always be 600s
			if thresholds.UploadErrorInterval != 600 {
				t.Errorf("UploadErrorInterval must be 600s, got %d", thresholds.UploadErrorInterval)
			}

			// Verify none of the thresholds are zero
			if thresholds.UploadOKInterval == 0 {
				t.Error("UploadOKInterval should not be zero for sub-second intervals")
			}

			if thresholds.UploadDegradedInterval == 0 {
				t.Error("UploadDegradedInterval should not be zero for sub-second intervals")
			}
		})
	}
}

func TestSubSecondIntervalHealthBehavior(t *testing.T) {
	// Test that sub-second intervals don't immediately mark uploader as degraded
	uploadInterval := 500 * time.Millisecond
	thresholds := ThresholdsFromUploadInterval(uploadInterval)
	checker := NewChecker(thresholds)

	now := time.Now()

	// Scenario: Last upload was 1 second ago with 500ms interval
	// Should be DEGRADED because 1s > 2s (OK threshold based on 1s minimum)
	// Actually, with 500ms -> 1s minimum, 2× = 2s, so 1s should be OK
	lastUploadTime := now.Add(-1 * time.Second)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 100)

	report := checker.GetReport()
	uploaderStatus := report.Components["uploader"]

	if uploaderStatus.Status != StatusOK {
		t.Errorf("With 500ms interval (1s minimum), 1s since upload should be OK, got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}

	// Scenario: Last upload was 3 seconds ago
	// Should be DEGRADED because 3s > 2s (OK threshold)
	lastUploadTime = now.Add(-3 * time.Second)
	checker.UpdateUploaderStatus(lastUploadTime, nil, 100)

	report = checker.GetReport()
	uploaderStatus = report.Components["uploader"]

	if uploaderStatus.Status != StatusDegraded {
		t.Errorf("With 500ms interval (1s minimum), 3s since upload should be DEGRADED, got %s (message: %s)",
			uploaderStatus.Status, uploaderStatus.Message)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
