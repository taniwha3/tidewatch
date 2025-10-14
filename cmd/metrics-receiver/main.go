package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
)

var (
	port    = flag.Int("port", 8080, "Port to listen on")
	verbose = flag.Bool("verbose", false, "Verbose logging")
	version = flag.Bool("version", false, "Print version and exit")
)

const appVersion = "1.0.0"

type MetricsPayload struct {
	DeviceID  string   `json:"device_id"`
	Timestamp string   `json:"timestamp"`
	Metrics   []Metric `json:"metrics"`
}

type Metric struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Timestamp string  `json:"timestamp"`
}

type Response struct {
	Success  bool   `json:"success"`
	Received int    `json:"received"`
	Error    string `json:"error,omitempty"`
}

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("tidewatch-metrics-receiver %s\n", appVersion)
		return
	}

	http.HandleFunc("/api/metrics", handleMetrics)
	http.HandleFunc("/health", handleHealth)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Metrics receiver starting on %s", addr)
	log.Printf("POST metrics to http://localhost%s/api/metrics", addr)
	log.Printf("Verbose logging: %t", *verbose)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload MetricsPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("Failed to decode payload: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   fmt.Sprintf("Invalid JSON: %v", err),
		})
		return
	}

	// Log received metrics
	now := time.Now().Format("15:04:05")
	log.Printf("[%s] Received %d metrics from %s (batch timestamp: %s)",
		now, len(payload.Metrics), payload.DeviceID, payload.Timestamp)

	if *verbose {
		for _, m := range payload.Metrics {
			log.Printf("  %s = %.2f @ %s", m.Name, m.Value, m.Timestamp)
		}
	}

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Success:  true,
		Received: len(payload.Metrics),
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": appVersion,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}
