package collector

import (
	"context"

	"github.com/taniwha3/tidewatch/internal/models"
)

// Collector is the interface for metric collectors
type Collector interface {
	// Name returns the collector's name
	Name() string

	// Collect gathers metrics and returns them
	Collect(ctx context.Context) ([]*models.Metric, error)
}
