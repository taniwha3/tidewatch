package watchdog

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
)

// Pinger sends periodic keepalive notifications to systemd watchdog
type Pinger struct {
	enabled  bool
	interval time.Duration
	logger   *slog.Logger
}

// NewPinger creates a new watchdog pinger
// It automatically detects if running under systemd with watchdog enabled
func NewPinger(logger *slog.Logger) *Pinger {
	// Check if systemd watchdog is enabled
	interval, err := daemon.SdWatchdogEnabled(false)
	if err != nil || interval == 0 {
		logger.Info("systemd watchdog not enabled, skipping watchdog notifications")
		return &Pinger{
			enabled: false,
			logger:  logger,
		}
	}

	// Ping at half the watchdog interval for safety margin
	pingInterval := interval / 2

	logger.Info("systemd watchdog enabled",
		"watchdog_timeout", interval,
		"ping_interval", pingInterval,
	)

	return &Pinger{
		enabled:  true,
		interval: pingInterval,
		logger:   logger,
	}
}

// Start begins the watchdog ping routine
// It runs until the context is cancelled
// Note: Does NOT send READY notification - call NotifyReady() explicitly after initialization
func (p *Pinger) Start(ctx context.Context) {
	if !p.enabled {
		return
	}

	// Start watchdog ping loop
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Send stopping notification before shutdown
			_, _ = daemon.SdNotify(false, daemon.SdNotifyStopping)
			p.logger.Info("watchdog pinger stopped")
			return

		case <-ticker.C:
			// Send watchdog keepalive
			sent, err := daemon.SdNotify(false, daemon.SdNotifyWatchdog)
			if err != nil {
				p.logger.Error("failed to send watchdog ping", "error", err)
			} else if sent {
				p.logger.Debug("watchdog ping sent")
			}
		}
	}
}

// NotifyReady sends a ready notification to systemd
// This is called when the service has finished initialization
func (p *Pinger) NotifyReady() {
	if !p.enabled {
		return
	}

	sent, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	if err != nil {
		p.logger.Error("failed to notify systemd ready", "error", err)
	} else if sent {
		p.logger.Info("notified systemd: service ready")
	}
}

// NotifyStopping sends a stopping notification to systemd
// This should be called before clean shutdown
func (p *Pinger) NotifyStopping() {
	if !p.enabled {
		return
	}

	sent, err := daemon.SdNotify(false, daemon.SdNotifyStopping)
	if err != nil {
		p.logger.Error("failed to notify systemd stopping", "error", err)
	} else if sent {
		p.logger.Info("notified systemd: service stopping")
	}
}

// IsEnabled returns whether watchdog is enabled
func (p *Pinger) IsEnabled() bool {
	return p.enabled
}

// GetInterval returns the ping interval
func (p *Pinger) GetInterval() time.Duration {
	return p.interval
}

// IsRunningUnderSystemd checks if the process is running under systemd
func IsRunningUnderSystemd() bool {
	// Check for NOTIFY_SOCKET environment variable
	if os.Getenv("NOTIFY_SOCKET") != "" {
		return true
	}
	// Check for INVOCATION_ID (set by systemd for all service units)
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	return false
}
