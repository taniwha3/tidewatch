package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/taniwha3/thugshells/internal/collector"
	"github.com/taniwha3/thugshells/internal/config"
	"github.com/taniwha3/thugshells/internal/health"
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

	log.Printf("Starting metrics collector for device: %s", cfg.Device.ID)
	log.Printf("Storage: %s", cfg.Storage.Path)
	log.Printf("Remote: %s (enabled: %t)", cfg.Remote.URL, cfg.Remote.Enabled)

	// Initialize storage
	store, err := storage.NewSQLiteStorage(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()
	log.Printf("Storage initialized")

	// Initialize health checker with thresholds derived from upload interval
	uploadInterval := cfg.Remote.UploadInterval()
	healthThresholds := health.ThresholdsFromUploadInterval(uploadInterval)
	healthChecker := health.NewChecker(healthThresholds)
	log.Printf("Health checker initialized (upload interval: %s, OK threshold: %ds, degraded: %ds, error: %ds)",
		uploadInterval, healthThresholds.UploadOKInterval, healthThresholds.UploadDegradedInterval, healthThresholds.UploadErrorInterval)

	// Initialize uploader (if remote enabled)
	var upload uploader.Uploader
	if cfg.Remote.Enabled {
		upload = uploader.NewHTTPUploader(cfg.Remote.URL, cfg.Device.ID)
		defer upload.Close()
		log.Printf("Uploader initialized")
	}

	// Initialize collectors
	collectors := initializeCollectors(cfg)
	log.Printf("Initialized %d collectors", len(collectors))

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
		log.Printf("Starting health server on %s", healthAddr)
		if err := healthChecker.StartHTTPServer(ctx, healthAddr); err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()

	// Start collection loops
	for name, coll := range collectors {
		wg.Add(1)
		go func(name string, c collector.Collector, interval time.Duration) {
			defer wg.Done()
			runCollector(ctx, name, c, interval, store, upload, healthChecker)
		}(name, coll.collector, coll.interval)
	}

	// Start upload loop (if remote enabled)
	if cfg.Remote.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runUploadLoop(ctx, store, upload, cfg.Remote.UploadInterval(), healthChecker)
		}()
	}

	// Start storage health monitoring loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		runStorageMonitoring(ctx, store, healthChecker)
	}()

	log.Printf("All collectors started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	log.Printf("Shutdown signal received, stopping...")

	// Cancel context to stop all goroutines
	cancel()

	// Wait for all goroutines to finish
	wg.Wait()

	log.Printf("Shutdown complete")
}

type collectorInfo struct {
	collector collector.Collector
	interval  time.Duration
}

// initializeCollectors creates and configures all enabled collectors
func initializeCollectors(cfg *config.Config) map[string]collectorInfo {
	collectors := make(map[string]collectorInfo)
	enabled := cfg.EnabledMetrics()

	for _, mc := range enabled {
		interval, err := mc.IntervalDuration()
		if err != nil {
			log.Printf("Invalid interval for %s: %v, skipping", mc.Name, err)
			continue
		}

		var coll collector.Collector

		switch mc.Name {
		case "cpu.temperature":
			coll = collector.NewSystemCollector(cfg.Device.ID)
		case "srt.packet_loss":
			coll = collector.NewMockSRTCollector(cfg.Device.ID)
		default:
			log.Printf("Unknown metric: %s, skipping", mc.Name)
			continue
		}

		collectors[mc.Name] = collectorInfo{
			collector: coll,
			interval:  interval,
		}

		log.Printf("Registered collector: %s (interval: %s)", mc.Name, interval)
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
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	collectAndStore(ctx, name, coll, store, healthChecker)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndStore(ctx, name, coll, store, healthChecker)
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
) {
	metrics, err := coll.Collect(ctx)

	// Update health status for this collector
	if healthChecker != nil {
		healthChecker.UpdateCollectorStatus(name, err, len(metrics))
	}

	if err != nil {
		log.Printf("[%s] Collection failed: %v", name, err)
		return
	}

	if len(metrics) == 0 {
		return
	}

	if err := store.StoreBatch(ctx, metrics); err != nil {
		log.Printf("[%s] Failed to store metrics: %v", name, err)
		return
	}

	log.Printf("[%s] Collected and stored %d metric(s)", name, len(metrics))
}

// runUploadLoop periodically uploads metrics to remote endpoint
func runUploadLoop(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
	interval time.Duration,
	healthChecker *health.Checker,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Upload loop started (interval: %s)", interval)

	lastUploadTime := time.Now()
	var lastUploadErr error

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := uploadMetrics(ctx, store, upload)
			if err == nil {
				lastUploadTime = time.Now()
				lastUploadErr = nil
			} else {
				lastUploadErr = err
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
func uploadMetrics(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
) error {
	// Query unuploaded metrics (limit to reasonable batch size)
	const batchSize = 2500
	metrics, err := store.QueryUnuploaded(ctx, batchSize)

	if err != nil {
		log.Printf("[upload] Failed to query unuploaded metrics: %v", err)
		return err
	}

	if len(metrics) == 0 {
		return nil
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
		log.Printf("[upload] Failed to upload %d metrics: %v", len(metrics), err)
		return err
	}

	// Mark metrics as uploaded after successful upload
	if len(metricIDs) > 0 {
		if err := store.MarkUploaded(ctx, metricIDs); err != nil {
			log.Printf("[upload] Warning: Failed to mark %d metrics as uploaded: %v", len(metricIDs), err)
			// Don't return error here - metrics were uploaded successfully
		}
	}

	log.Printf("[upload] Uploaded and marked %d metric(s) as uploaded", len(metrics))
	return nil
}

// runStorageMonitoring periodically updates storage health metrics
func runStorageMonitoring(
	ctx context.Context,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Update immediately on start
	updateStorageHealth(ctx, store, healthChecker)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updateStorageHealth(ctx, store, healthChecker)
		}
	}
}

// updateStorageHealth updates storage-related health metrics
func updateStorageHealth(
	ctx context.Context,
	store *storage.SQLiteStorage,
	healthChecker *health.Checker,
) {
	if healthChecker == nil {
		return
	}

	dbSize, err := store.DBSize()
	if err != nil {
		log.Printf("[health] Failed to get DB size: %v", err)
		dbSize = 0
	}

	walSize, err := store.GetWALSize()
	if err != nil {
		log.Printf("[health] Failed to get WAL size: %v", err)
		walSize = 0
	}

	pendingCount, err := store.GetPendingCount(ctx)
	if err != nil {
		log.Printf("[health] Failed to get pending count: %v", err)
		pendingCount = 0
	}

	healthChecker.UpdateStorageStatus(dbSize, walSize, pendingCount)
}
