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

	// Start collection loops
	var wg sync.WaitGroup
	for name, coll := range collectors {
		wg.Add(1)
		go func(name string, c collector.Collector, interval time.Duration) {
			defer wg.Done()
			runCollector(ctx, name, c, interval, store, upload)
		}(name, coll.collector, coll.interval)
	}

	// Start upload loop (if remote enabled)
	if cfg.Remote.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runUploadLoop(ctx, store, upload, cfg.Remote.UploadInterval())
		}()
	}

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
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	collectAndStore(ctx, name, coll, store)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndStore(ctx, name, coll, store)
		}
	}
}

// collectAndStore collects metrics and stores them
func collectAndStore(
	ctx context.Context,
	name string,
	coll collector.Collector,
	store *storage.SQLiteStorage,
) {
	metrics, err := coll.Collect(ctx)
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
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Upload loop started (interval: %s)", interval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			uploadMetrics(ctx, store, upload)
		}
	}
}

// uploadMetrics queries recent metrics and uploads them
func uploadMetrics(
	ctx context.Context,
	store *storage.SQLiteStorage,
	upload uploader.Uploader,
) {
	// Query metrics from last 5 minutes
	endTime := time.Now()
	startTime := endTime.Add(-5 * time.Minute)

	metrics, err := store.Query(ctx, storage.QueryOptions{
		StartMs: startTime.UnixMilli(),
		EndMs:   endTime.UnixMilli(),
	})

	if err != nil {
		log.Printf("[upload] Failed to query metrics: %v", err)
		return
	}

	if len(metrics) == 0 {
		return
	}

	// Upload metrics
	if err := upload.Upload(ctx, metrics); err != nil {
		log.Printf("[upload] Failed to upload %d metrics: %v", len(metrics), err)
		return
	}

	log.Printf("[upload] Uploaded %d metric(s)", len(metrics))
}
