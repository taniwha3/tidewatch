package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/taniwha3/tidewatch/internal/collector"
	"github.com/taniwha3/tidewatch/internal/config"
	"github.com/taniwha3/tidewatch/internal/health"
	"github.com/taniwha3/tidewatch/internal/lockfile"
	"github.com/taniwha3/tidewatch/internal/logging"
	"github.com/taniwha3/tidewatch/internal/monitoring"
	"github.com/taniwha3/tidewatch/internal/storage"
	"github.com/taniwha3/tidewatch/internal/uploader"
	"github.com/taniwha3/tidewatch/internal/watchdog"
)

var (
	configPath = flag.String("config", "/etc/tidewatch/config.yaml", "Path to config file")
	version    = flag.Bool("version", false, "Print version and exit")
	appVersion = "dev" // Set by -ldflags during build
)

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("tidewatch %s\n", appVersion)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Initialize structured logging
	logLevel := logging.LevelInfo
	if cfg.Logging.Level != "" {
		logLevel = logging.Level(cfg.Logging.Level)
	}
	logFormat := logging.FormatConsole
	if cfg.Logging.Format != "" {
		logFormat = logging.Format(cfg.Logging.Format)
	}

	logger := logging.New(logging.Config{
		Level:  logLevel,
		Format: logFormat,
		Output: os.Stdout,
	})
	logging.SetDefault(logger)

	logger.Info("Starting metrics collector",
		slog.String("device_id", cfg.Device.ID),
		slog.String("version", appVersion),
	)
	logger.Info("Configuration loaded",
		slog.String("storage_path", cfg.Storage.Path),
		slog.String("remote_url", cfg.Remote.URL),
		slog.Bool("remote_enabled", cfg.Remote.Enabled),
		slog.String("log_level", string(logLevel)),
		slog.String("log_format", string(logFormat)),
	)

	// Acquire process lock to prevent multiple instances
	// Normalize storage path to handle SQLite DSN URIs (e.g., file:///path?params)
	lockPath := lockfile.GetLockPath(normalizeStoragePath(cfg.Storage.Path))
	lock, err := lockfile.Acquire(lockPath)
	if err != nil {
		logger.Error("Failed to acquire process lock - another instance may be running",
			slog.Any("error", err),
			slog.String("lock_path", lockPath),
		)
		os.Exit(1)
	}
	defer lock.Release()
	logger.Info("Process lock acquired", slog.String("lock_path", lockPath))

	// Initialize watchdog
	wd := watchdog.NewPinger(logger)

	// Create context for watchdog and other goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watchdog ping routine in background (if watchdog is enabled)
	// Note: This is separate from systemd Type=notify - we always send READY/STOPPING
	// when running under systemd, but only send periodic watchdog pings if configured
	if wd.IsEnabled() {
		go wd.Start(ctx)
		logger.Info("Watchdog pinger started", slog.Duration("interval", wd.GetInterval()))
	}

	// Initialize storage
	store, err := storage.NewSQLiteStorage(cfg.Storage.Path)
	if err != nil {
		logger.Error("Failed to initialize storage", slog.Any("error", err))
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("Storage initialized")

	// Start WAL checkpoint routine
	// Context for WAL checkpoint routine (separate from main context for shutdown control)
	walCtx, walCancel := context.WithCancel(context.Background())
	defer walCancel()

	// Start WAL checkpoint routine with configured interval and size threshold
	walCheckpointInterval, err := cfg.Storage.WALCheckpointInterval()
	if err != nil {
		// This should never happen since Validate() already checked it
		logger.Error("Invalid WAL checkpoint interval", slog.Any("error", err))
		os.Exit(1)
	}
	walCheckpointSize := cfg.Storage.WALCheckpointSizeBytes()
	cancelWAL := store.StartWALCheckpointRoutine(walCtx, logger, walCheckpointInterval, walCheckpointSize)
	defer cancelWAL()
	logger.Info("WAL checkpoint routine configured",
		slog.Duration("interval", walCheckpointInterval),
		slog.Int64("size_threshold_bytes", walCheckpointSize),
	)

	// Initialize meta-metrics collector
	metricsCollector := monitoring.NewMetricsCollector(cfg.Device.ID)
	logger.Info("Meta-metrics collector initialized")

	// Initialize health checker with thresholds derived from upload interval
	uploadInterval, err := cfg.Remote.UploadInterval()
	if err != nil {
		// This should never happen since Validate() already checked it
		logger.Error("Invalid upload interval", slog.Any("error", err))
		os.Exit(1)
	}
	healthThresholds := health.ThresholdsFromUploadInterval(uploadInterval)

	// Override clock skew threshold if configured
	if cfg.Monitoring.ClockSkewWarnThresholdMs > 0 {
		healthThresholds.ClockSkewThresholdMs = int64(cfg.Monitoring.ClockSkewWarnThresholdMs)
	}

	healthChecker := health.NewChecker(healthThresholds)
	logger.Info("Health checker initialized",
		slog.Duration("upload_interval", uploadInterval),
		slog.Int("ok_threshold_sec", healthThresholds.UploadOKInterval),
		slog.Int("degraded_threshold_sec", healthThresholds.UploadDegradedInterval),
		slog.Int("error_threshold_sec", healthThresholds.UploadErrorInterval),
		slog.Int64("clock_skew_threshold_ms", healthThresholds.ClockSkewThresholdMs),
	)

	// Initialize uploader (if remote enabled)
	var upload uploader.Uploader
	if cfg.Remote.Enabled {
		// Build uploader config from remote settings
		uploaderCfg := uploader.HTTPUploaderConfig{
			URL:       cfg.Remote.URL,
			DeviceID:  cfg.Device.ID,
			AuthToken: cfg.Remote.AuthToken,
			// Set timeout explicitly to avoid default logic
			Timeout: 30 * time.Second,
		}

		// Apply retry configuration only if explicitly configured
		// Check if retry block is configured at all (Enabled field set OR any numeric field non-zero)
		retryConfigured := cfg.Remote.Retry.Enabled != nil ||
			cfg.Remote.Retry.MaxAttempts > 0 ||
			cfg.Remote.Retry.InitialBackoffStr != "" ||
			cfg.Remote.Retry.MaxBackoffStr != "" ||
			cfg.Remote.Retry.BackoffMultiplier > 0 ||
			cfg.Remote.Retry.JitterPercent != nil

		if retryConfigured {
			// Retry block is configured - honor the enabled flag
			// Default to true if Enabled is nil but other fields are set
			enabled := cfg.Remote.Retry.Enabled == nil || *cfg.Remote.Retry.Enabled
			if enabled {
				// Use configured retry values, applying defaults for unset fields
				maxAttempts := cfg.Remote.Retry.MaxAttempts
				if maxAttempts == 0 {
					// User enabled retries but didn't set max_attempts - use default
					maxAttempts = 3
				}
				// Convert max_attempts (total attempts) to maxRetries (number of retries)
				// max_attempts=1 → maxRetries=0 (1 attempt, no retries)
				// max_attempts=3 → maxRetries=2 (3 attempts = initial + 2 retries)
				maxRetries := maxAttempts - 1
				if maxRetries < 0 {
					maxRetries = 0
				}
				uploaderCfg.MaxRetries = &maxRetries

				retryDelay, err := cfg.Remote.Retry.InitialBackoff()
				if err != nil {
					// This should never happen since Validate() already checked it
					logger.Error("Invalid retry initial_backoff", slog.Any("error", err))
					os.Exit(1)
				}
				maxBackoff, err := cfg.Remote.Retry.MaxBackoff()
				if err != nil {
					// This should never happen since Validate() already checked it
					logger.Error("Invalid retry max_backoff", slog.Any("error", err))
					os.Exit(1)
				}
				uploaderCfg.RetryDelay = retryDelay
				uploaderCfg.MaxBackoff = maxBackoff
				uploaderCfg.BackoffMultiplier = cfg.Remote.Retry.BackoffMultiplier

				// JitterPercent: nil means use default (20), otherwise honor the value (even if 0)
				if cfg.Remote.Retry.JitterPercent != nil {
					// User explicitly set jitter_percent - honor it (even if 0)
					uploaderCfg.JitterPercent = cfg.Remote.Retry.JitterPercent
				} else {
					// User enabled retries but didn't set jitter_percent - use default
					// Critical: Without jitter, all instances retry in lockstep (thundering herd)
					jitter := 20
					uploaderCfg.JitterPercent = &jitter
				}
			} else {
				// Explicitly disabled - set MaxRetries=0 (means 1 attempt, no retries)
				zero := 0
				uploaderCfg.MaxRetries = &zero
				uploaderCfg.RetryDelay = 1 * time.Second
				uploaderCfg.JitterPercent = &zero
			}
		}
		// Otherwise: retry block not configured at all
		// Leave MaxRetries and JitterPercent as nil (uploader will use defaults)

		// Apply chunk size configuration
		uploaderCfg.ChunkSize = cfg.Remote.GetChunkSize()

		upload = uploader.NewHTTPUploaderWithConfig(uploaderCfg)
		defer upload.Close()

		// Log uploader config
		if retryConfigured {
			maxRetries := 3 // default
			if uploaderCfg.MaxRetries != nil {
				maxRetries = *uploaderCfg.MaxRetries
			}
			jitterPercent := 20 // default
			if uploaderCfg.JitterPercent != nil {
				jitterPercent = *uploaderCfg.JitterPercent
			}
			logger.Info("Uploader initialized",
				slog.Int("chunk_size", uploaderCfg.ChunkSize),
				slog.Int("max_retries", maxRetries),
				slog.Duration("retry_delay", uploaderCfg.RetryDelay),
				slog.Duration("max_backoff", uploaderCfg.MaxBackoff),
				slog.Float64("backoff_multiplier", uploaderCfg.BackoffMultiplier),
				slog.Int("jitter_percent", jitterPercent),
				slog.Bool("retry_configured", true),
			)
		} else {
			logger.Info("Uploader initialized",
				slog.Int("chunk_size", uploaderCfg.ChunkSize),
				slog.String("retry_config", "using defaults (3 retries, 1s initial, 30s max, 2.0x multiplier, 20% jitter)"),
			)
		}
	}

	// Initialize collectors
	collectors := initializeCollectors(cfg, logger)
	logger.Info("Collectors initialized", slog.Int("count", len(collectors)))

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// WaitGroup for coordinating goroutine shutdown
	var wg sync.WaitGroup

	// Start health server
	healthAddr := cfg.Monitoring.HealthAddress
	if healthAddr == "" {
		healthAddr = ":9100" // Default port
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting health server", slog.String("address", healthAddr))
		if err := healthChecker.StartHTTPServer(ctx, healthAddr); err != nil {
			logger.Error("Health server error", slog.Any("error", err))
		}
	}()

	// Start collection loops
	for name, coll := range collectors {
		wg.Add(1)
		go func(name string, c collector.Collector, interval time.Duration) {
			defer wg.Done()
			runCollector(ctx, name, c, interval, store, upload, healthChecker, metricsCollector, logger)
		}(name, coll.collector, coll.interval)
	}

	// Start upload loop (if remote enabled)
	if cfg.Remote.Enabled {
		uploadInterval, err = cfg.Remote.UploadInterval()
		if err != nil {
			// This should never happen since Validate() already checked it
			logger.Error("Invalid upload interval", slog.Any("error", err))
			os.Exit(1)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			runUploadLoop(ctx, store, upload, uploadInterval, cfg.Remote.GetBatchSize(), healthChecker, metricsCollector, logger)
		}()
	}

	// Start storage health monitoring loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		runStorageMonitoring(ctx, store, healthChecker, metricsCollector, logger)
	}()

	// Start meta-metrics collection loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		runMetaMetricsLoop(ctx, store, metricsCollector, 60*time.Second, logger)
	}()

	// Start clock skew checking routine (if configured)
	if cfg.Monitoring.ClockSkewURL != "" {
		clockCheckInterval := 5 * time.Minute // Default to 5 minutes
		if cfg.Monitoring.ClockSkewCheckInterval != "" {
			if interval, err := time.ParseDuration(cfg.Monitoring.ClockSkewCheckInterval); err == nil {
				// Guard against non-positive intervals to prevent panic in time.NewTicker
				// Non-positive durations (0s, -1s, etc.) fall back to the default
				if interval > 0 {
					clockCheckInterval = interval
				} else {
					logger.Warn("Clock skew check interval must be positive, using default",
						slog.Duration("configured", interval),
						slog.Duration("default", 5*time.Minute),
					)
				}
			}
		}

		warnThresholdMs := int64(2000) // Default to 2000ms
		if cfg.Monitoring.ClockSkewWarnThresholdMs > 0 {
			warnThresholdMs = int64(cfg.Monitoring.ClockSkewWarnThresholdMs)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			runClockSkewLoop(ctx, cfg, store, healthChecker, metricsCollector, clockCheckInterval, warnThresholdMs, logger)
		}()
	}

	// Notify systemd that service is ready (required for Type=notify)
	// Always send READY when running under systemd, even if watchdog is disabled
	if watchdog.IsRunningUnderSystemd() {
		sent, err := daemon.SdNotify(false, daemon.SdNotifyReady)
		if err != nil {
			logger.Error("Failed to notify systemd ready", slog.Any("error", err))
		} else if sent {
			logger.Info("Notified systemd: service ready")
		}
	}
	logger.Info("All collectors started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutdown signal received, stopping...")

	// Notify systemd we're stopping (send even if watchdog is disabled)
	if watchdog.IsRunningUnderSystemd() {
		sent, err := daemon.SdNotify(false, daemon.SdNotifyStopping)
		if err != nil {
			logger.Error("Failed to notify systemd stopping", slog.Any("error", err))
		} else if sent {
			logger.Info("Notified systemd: service stopping")
		}
	}

	// Cancel context to stop all goroutines
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()

	logger.Info("Shutdown complete")
}

type collectorInfo struct {
	collector collector.Collector
	interval  time.Duration
}

// initializeCollectors creates and configures all enabled collectors
func initializeCollectors(cfg *config.Config, logger *slog.Logger) map[string]collectorInfo {
	collectors := make(map[string]collectorInfo)
	enabled := cfg.EnabledMetrics()

	for _, mc := range enabled {
		interval, err := mc.IntervalDuration()
		if err != nil {
			logger.Warn("Invalid interval, skipping collector",
				slog.String("collector", mc.Name),
				slog.Any("error", err),
			)
			continue
		}

		var coll collector.Collector

		switch mc.Name {
		case "cpu.temperature":
			coll = collector.NewSystemCollector(cfg.Device.ID)
		case "cpu.usage":
			coll = collector.NewCPUCollector(cfg.Device.ID)
		case "memory.usage":
			coll = collector.NewMemoryCollector(cfg.Device.ID)
		case "disk.io":
			coll = collector.NewDiskCollector(cfg.Device.ID)
		case "network.traffic":
			coll = collector.NewNetworkCollector(cfg.Device.ID)
		case "srt.packet_loss":
			coll = collector.NewMockSRTCollector(cfg.Device.ID)
		default:
			logger.Warn("Unknown metric, skipping", slog.String("collector", mc.Name))
			continue
		}

		collectors[mc.Name] = collectorInfo{
			collector: coll,
			interval:  interval,
		}

		logger.Info("Registered collector",
			slog.String("collector", mc.Name),
			slog.Duration("interval", interval),
		)
	}

	return collectors
}

// runCollector runs a single collector in a loop
func runCollector(
	ctx context.Context,
	name string,
	coll collector.Collector,
	interval time.Duration,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	collectAndStore(ctx, name, coll, store, healthChecker, metricsCollector, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndStore(ctx, name, coll, store, healthChecker, metricsCollector, logger)
		}
	}
}

// collectAndStore collects metrics and stores them
func collectAndStore(
	ctx context.Context,
	name string,
	coll collector.Collector,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	startTime := time.Now()
	metrics, err := coll.Collect(ctx)
	collectionDuration := time.Since(startTime)

	// Handle collection errors
	if err != nil {
		// Update health status
		if healthChecker != nil {
			healthChecker.UpdateCollectorStatus(name, err, 0)
		}
		// Record failure in meta-metrics
		if metricsCollector != nil {
			metricsCollector.RecordCollectionFailure(name)
		}
		logger.Error("Collection failed",
			slog.String("collector", name),
			slog.Any("error", err),
			slog.Int64("duration_ms", collectionDuration.Milliseconds()),
		)
		return
	}

	if len(metrics) == 0 {
		// No metrics collected - update health but don't record as failure
		if healthChecker != nil {
			healthChecker.UpdateCollectorStatus(name, nil, 0)
		}
		return
	}

	// Attempt to store metrics
	storageStartTime := time.Now()
	if err := store.StoreBatch(ctx, metrics); err != nil {
		// Storage failed - treat as collection failure
		if healthChecker != nil {
			healthChecker.UpdateCollectorStatus(name, err, len(metrics))
		}
		if metricsCollector != nil {
			metricsCollector.RecordCollectionFailure(name)
		}
		logger.Error("Failed to store metrics",
			slog.String("collector", name),
			slog.Int("count", len(metrics)),
			slog.Any("error", err),
		)
		return
	}

	// Success - record meta-metrics with total duration (collect + store)
	totalDuration := collectionDuration + time.Since(storageStartTime)
	if healthChecker != nil {
		healthChecker.UpdateCollectorStatus(name, nil, len(metrics))
	}
	if metricsCollector != nil {
		metricsCollector.RecordCollectionSuccess(name, len(metrics), totalDuration)
	}

	logger.Info("Collection completed",
		slog.String("collector", name),
		slog.Int("count", len(metrics)),
		slog.Int64("duration_ms", totalDuration.Milliseconds()),
	)
}

// runUploadLoop periodically uploads metrics to remote endpoint
func runUploadLoop(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
	interval time.Duration,
	batchSize int,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Upload loop started",
		slog.Duration("interval", interval),
		slog.Int("batch_size", batchSize),
	)

	lastUploadTime := time.Now()
	var lastUploadErr error

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			startTime := time.Now()
			count, err := uploadMetrics(ctx, store, upload, batchSize, logger)
			duration := time.Since(startTime)

			if err == nil {
				lastUploadTime = time.Now()
				lastUploadErr = nil
				if metricsCollector != nil && count > 0 {
					metricsCollector.RecordUploadSuccess(count, duration)
				}
			} else {
				lastUploadErr = err
				if metricsCollector != nil {
					metricsCollector.RecordUploadFailure()
				}
			}

			// Update health status
			if healthChecker != nil {
				pendingCount, _ := store.GetPendingCount(ctx)
				healthChecker.UpdateUploaderStatus(lastUploadTime, lastUploadErr, pendingCount)
			}
		}
	}
}

// uploadMetrics queries unuploaded metrics and uploads them
// Returns the number of metrics actually sent to VictoriaMetrics (numeric only) and any error
// Note: String metrics are processed and marked as uploaded but not counted in the return value
func uploadMetrics(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
	batchSize int,
	logger *slog.Logger,
) (int, error) {
	// Query unuploaded metrics (limit to configured batch size)
	metrics, err := store.QueryUnuploaded(ctx, batchSize)

	if err != nil {
		logger.Error("Failed to query unuploaded metrics", slog.Any("error", err))
		return 0, err
	}

	if len(metrics) == 0 {
		return 0, nil
	}

	// Upload metrics and collect IDs of metrics actually sent to VictoriaMetrics
	var uploadedIDs []int64

	// Try to use UploadAndGetIDs if available (for HTTPUploader)
	if httpUploader, ok := upload.(*uploader.HTTPUploader); ok {
		var err error
		uploadedIDs, err = httpUploader.UploadAndGetIDs(ctx, metrics)
		if err != nil {
			logger.Error("Upload failed",
				slog.Int("count", len(metrics)),
				slog.Any("error", err),
			)
			return 0, err
		}
	} else {
		// Fallback for other uploaders (e.g., mocks): upload and extract IDs manually
		if err := upload.Upload(ctx, metrics); err != nil {
			logger.Error("Upload failed",
				slog.Int("count", len(metrics)),
				slog.Any("error", err),
			)
			return 0, err
		}

		// Extract IDs from all metrics in the batch (already filtered to numeric only by QueryUnuploaded)
		for _, m := range metrics {
			if idStr, ok := m.Tags["_storage_id"]; ok {
				var id int64
				if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
					uploadedIDs = append(uploadedIDs, id)
				}
			}
		}
	}

	// Mark only the metrics that were actually uploaded (numeric metrics only)
	// String metrics never enter this function because QueryUnuploaded filters them out
	// They remain in SQLite with uploaded=0 for local event processing
	if len(uploadedIDs) > 0 {
		if err := store.MarkUploaded(ctx, uploadedIDs); err != nil {
			logger.Warn("Failed to mark metrics as uploaded",
				slog.Int("count", len(uploadedIDs)),
				slog.Any("error", err),
			)
			// Don't return error here - metrics were uploaded successfully
		}
	}

	logger.Info("Upload completed",
		slog.Int("count", len(uploadedIDs)),
	)

	// Return the count of metrics actually sent to VictoriaMetrics
	// This ensures meta-metrics accurately reflect what was uploaded
	return len(uploadedIDs), nil
}

// runStorageMonitoring periodically updates storage health metrics
func runStorageMonitoring(
	ctx context.Context,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Update immediately on start
	updateStorageHealth(ctx, store, healthChecker, metricsCollector, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updateStorageHealth(ctx, store, healthChecker, metricsCollector, logger)
		}
	}
}

// updateStorageHealth updates storage-related health metrics
func updateStorageHealth(
	ctx context.Context,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	dbSize, err := store.DBSize()
	if err != nil {
		logger.Error("Failed to get DB size", slog.Any("error", err))
		dbSize = 0
	}

	walSize, err := store.GetWALSize()
	if err != nil {
		logger.Error("Failed to get WAL size", slog.Any("error", err))
		walSize = 0
	}

	pendingCount, err := store.GetPendingCount(ctx)
	if err != nil {
		logger.Error("Failed to get pending count", slog.Any("error", err))
		pendingCount = 0
	}

	// Update health checker
	if healthChecker != nil {
		healthChecker.UpdateStorageStatus(dbSize, walSize, pendingCount)
	}

	// Update meta-metrics collector
	if metricsCollector != nil {
		metricsCollector.UpdateStorageMetrics(dbSize, walSize, pendingCount)
	}
}

// runMetaMetricsLoop periodically collects and stores meta-metrics
func runMetaMetricsLoop(
	ctx context.Context,
	store *storage.SQLiteStorage,
	metricsCollector *monitoring.MetricsCollector,
	interval time.Duration,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Meta-metrics collection loop started", slog.Duration("interval", interval))

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect meta-metrics
			metaMetrics, err := metricsCollector.CollectMetrics(ctx)
			if err != nil {
				logger.Error("Failed to collect meta-metrics", slog.Any("error", err))
				continue
			}

			if len(metaMetrics) == 0 {
				continue
			}

			// Store meta-metrics
			if err := store.StoreBatch(ctx, metaMetrics); err != nil {
				logger.Error("Failed to store meta-metrics",
					slog.Int("count", len(metaMetrics)),
					slog.Any("error", err),
				)
				continue
			}

			logger.Debug("Meta-metrics collected and stored",
				slog.Int("count", len(metaMetrics)),
			)
		}
	}
}

// runClockSkewLoop periodically checks for clock skew and updates health status
func runClockSkewLoop(
	ctx context.Context,
	cfg *config.Config,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	interval time.Duration,
	warnThresholdMs int64,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Clock skew checking loop started",
		slog.Duration("interval", interval),
		slog.Int64("warn_threshold_ms", warnThresholdMs),
		slog.String("url", cfg.Monitoring.ClockSkewURL),
	)

	// Create clock skew collector
	// Note: Reuse auth token from remote config since clock skew endpoint
	// typically shares the same auth requirements as ingestion endpoint
	clockCollector := collector.NewClockSkewCollector(collector.ClockSkewCollectorConfig{
		DeviceID:        cfg.Device.ID,
		ClockSkewURL:    cfg.Monitoring.ClockSkewURL,
		AuthToken:       cfg.Remote.AuthToken, // Reuse auth token from remote config
		WarnThresholdMs: warnThresholdMs,
	})

	// Check immediately on start
	checkClockSkew(ctx, clockCollector, store, healthChecker, metricsCollector, logger)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkClockSkew(ctx, clockCollector, store, healthChecker, metricsCollector, logger)
		}
	}
}

// checkClockSkew performs a single clock skew check
func checkClockSkew(
	ctx context.Context,
	clockCollector collector.Collector,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	metrics, err := clockCollector.Collect(ctx)
	if err != nil {
		logger.Warn("Clock skew check failed", slog.Any("error", err))
		// Update health with error
		if healthChecker != nil {
			healthChecker.UpdateClockSkewStatus(0, err)
		}
		return
	}

	// Extract skew value from metrics
	var skewMs int64
	if len(metrics) > 0 {
		skewMs = int64(metrics[0].Value)
	}

	// Store metrics
	if len(metrics) > 0 {
		if err := store.StoreBatch(ctx, metrics); err != nil {
			logger.Error("Failed to store clock skew metrics",
				slog.Int("count", len(metrics)),
				slog.Any("error", err),
			)
		}
	}

	// Update health status
	if healthChecker != nil {
		healthChecker.UpdateClockSkewStatus(skewMs, nil)
	}

	// Update meta-metrics collector
	if metricsCollector != nil {
		metricsCollector.UpdateTimeSkew(skewMs)
	}

	logger.Debug("Clock skew check completed", slog.Int64("skew_ms", skewMs))
}

// normalizeStoragePath converts a storage path (including SQLite URIs) to an absolute file path
// suitable for lock file generation. This handles SQLite DSN URIs like:
//   - file:/var/lib/tidewatch/metrics.db?cache=shared
//   - file:///var/lib/tidewatch/metrics.db
//   - ./data/metrics.db
//   - /var/lib/tidewatch/metrics.db
func normalizeStoragePath(storagePath string) string {
	// Handle SQLite URI format (file:...)
	if strings.HasPrefix(storagePath, "file:") {
		// Strip "file:" prefix
		path := strings.TrimPrefix(storagePath, "file:")

		// Strip any query parameters (everything after ?)
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}

		// Handle SQLite URI formats:
		// - file:///path (three slashes) -> /path
		// - file://host/path (two slashes + host) -> skip for now, treat as //host/path
		// - file:/path (one slash) -> /path
		// - file:path (no slash) -> path (relative)
		if strings.HasPrefix(path, "///") {
			// file:///absolute/path -> /absolute/path
			path = path[2:] // Remove two slashes, keep the third as leading /
		} else if strings.HasPrefix(path, "//") {
			// file://hostname/path - this is a UNC network path (e.g., //host/share/file.db)
			// Preserve the // prefix so the lock file is created at the correct UNC location
			// This ensures multiple instances accessing the same network database coordinate properly
			// path remains as //hostname/path
		}
		// Now path is either /absolute or relative

		// Make relative paths absolute
		if !strings.HasPrefix(path, "/") {
			absPath, err := filepath.Abs(path)
			if err == nil {
				return absPath
			}
		}

		return path
	}

	// For non-URI paths, make them absolute if they're relative
	if !strings.HasPrefix(storagePath, "/") {
		absPath, err := filepath.Abs(storagePath)
		if err == nil {
			return absPath
		}
	}

	return storagePath
}
