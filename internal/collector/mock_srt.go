package collector

import (
	"context"
	"math/rand"

	"github.com/taniwha3/thugshells/internal/models"
)

// MockSRTCollector generates mock SRT packet loss metrics
// Real SRT stats will come from server-side collector in Milestone 2
type MockSRTCollector struct {
	deviceID string
	rng      *rand.Rand
}

// NewMockSRTCollector creates a new mock SRT collector
func NewMockSRTCollector(deviceID string) *MockSRTCollector {
	return &MockSRTCollector{
		deviceID: deviceID,
		rng:      rand.New(rand.NewSource(int64(len(deviceID)))),
	}
}

// Name returns the collector name
func (c *MockSRTCollector) Name() string {
	return "mock_srt"
}

// Collect generates mock SRT packet loss metric
func (c *MockSRTCollector) Collect(ctx context.Context) ([]*models.Metric, error) {
	// Simulate occasional packet loss
	var packetLoss float64
	if c.rng.Float64() < 0.1 { // 10% chance of packet loss
		packetLoss = c.rng.Float64() * 5.0 // 0-5% loss
	}

	m := models.NewMetric("srt.packet_loss_pct", packetLoss, c.deviceID)

	return []*models.Metric{m}, nil
}
