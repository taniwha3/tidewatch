package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/taniwha3/thugshells/internal/collector"
	"github.com/taniwha3/thugshells/internal/config"
	"github.com/taniwha3/thugshells/internal/health"
	"github.com/taniwha3/thugshells/internal/logging"
	"github.com/taniwha3/thugshells/internal/monitoring"
	"github.com/taniwha3/thugshells/internal/storage"
	"github.com/taniwha3/thugshells/internal/uploader"
)

var (
	configPath = flag.String("config", "/etc/belabox-metrics/config.yaml", "Path to config file")
	version    = flag.Bool("version", false, "Print version and exit")
)

const appVersion = "1.0.0"

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("thugshells-metrics-collector %s\n", appVersion)
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

	// Start WAL checkpoint routine with 1 hour interval and 64 MB threshold
	cancelWAL := store.StartWALCheckpointRoutine(walCtx, logger, 1*time.Hour, 64*1024*1024)
	defer cancelWAL()

	// Initialize meta-metrics collector
	metricsCollector := monitoring.NewMetricsCollector(cfg.Device.ID)
	logger.Info("Meta-metrics collector initialized")

	// Initialize health checker with thresholds derived from upload interval
	uploadInterval := cfg.Remote.UploadInterval()
	healthThresholds := health.ThresholdsFromUploadInterval(uploadInterval)
	healthChecker := health.NewChecker(healthThresholds)
	logger.Info("Health checker initialized",
		slog.Duration("upload_interval", uploadInterval),
		slog.Int("ok_threshold_sec", healthThresholds.UploadOKInterval),
		slog.Int("degraded_threshold_sec", healthThresholds.UploadDegradedInterval),
		slog.Int("error_threshold_sec", healthThresholds.UploadErrorInterval),
	)

	// Initialize uploader (if remote enabled)
	var upload uploader.Uploader
	if cfg.Remote.Enabled {
		upload = uploader.NewHTTPUploader(cfg.Remote.URL, cfg.Device.ID)
		defer upload.Close()
		logger.Info("Uploader initialized")
	}

	// Initialize collectors
	collectors := initializeCollectors(cfg, logger)
	logger.Info("Collectors initialized", slog.Int("count", len(collectors)))

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
		wg.Add(1)
		go func() {
			defer wg.Done()
			runUploadLoop(ctx, store, upload, cfg.Remote.UploadInterval(), healthChecker, metricsCollector, logger)
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

	logger.Info("All collectors started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutdown signal received, stopping...")

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
	healthChecker *health.Checker,
	metricsCollector *monitoring.MetricsCollector,
	logger *slog.Logger,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Upload loop started", slog.Duration("interval", interval))

	lastUploadTime := time.Now()
	var lastUploadErr error

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			startTime := time.Now()
			count, err := uploadMetrics(ctx, store, upload, logger)
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
// Returns the number of metrics uploaded and any error
func uploadMetrics(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
	logger *slog.Logger,
) (int, error) {
	// Query unuploaded metrics (limit to reasonable batch size)
	const batchSize = 2500
	metrics, err := store.QueryUnuploaded(ctx, batchSize)

	if err != nil {
		logger.Error("Failed to query unuploaded metrics", slog.Any("error", err))
		return 0, err
	}

	if len(metrics) == 0 {
		return 0, nil
	}

	// Extract metric IDs from the _storage_id tag for marking as uploaded
	metricIDs := make([]int64, 0, len(metrics))
	for _, m := range metrics {
		if idStr, ok := m.Tags["_storage_id"]; ok {
			var id int64
			if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
				metricIDs = append(metricIDs, id)
			}
		}
	}

	// Upload metrics
	if err := upload.Upload(ctx, metrics); err != nil {
		logger.Error("Upload failed",
			slog.Int("count", len(metrics)),
			slog.Any("error", err),
		)
		return 0, err
	}

	// Mark metrics as uploaded after successful upload
	if len(metricIDs) > 0 {
		if err := store.MarkUploaded(ctx, metricIDs); err != nil {
			logger.Warn("Failed to mark metrics as uploaded",
				slog.Int("count", len(metricIDs)),
				slog.Any("error", err),
			)
			// Don't return error here - metrics were uploaded successfully
		}
	}

	logger.Info("Upload completed",
		slog.Int("count", len(metrics)),
	)
	return len(metrics), nil
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
