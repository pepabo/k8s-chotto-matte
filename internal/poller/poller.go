// Package poller implements the polling loop shared by every check,
// regardless of which connector.Connector backs it.
package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pepabo/k8s-chotto-matte/internal/connector"
)

// ErrCanceled is returned by Run when ctx is canceled before the connector
// has passed Config.SuccessThreshold consecutive times.
var ErrCanceled = errors.New("poller: canceled before check passed")

// Config holds the common monitoring-loop settings applied to a single
// connector.Connector.
type Config struct {
	// Name identifies the check for logging purposes.
	Name string
	// Interval is how often the connector is checked.
	Interval time.Duration
	// SuccessThreshold is how many consecutive passing checks are required
	// before Run returns successfully.
	SuccessThreshold int
	// Timeout bounds each individual Check call.
	Timeout time.Duration
	// FailOpen controls whether a Check error counts as a pass or a fail.
	FailOpen bool
}

// Run polls conn at the configured interval until it has passed
// Config.SuccessThreshold consecutive times, or ctx is canceled.
func Run(ctx context.Context, conn connector.Connector, cfg Config, logger *slog.Logger) error {
	consecutive := 0

	for {
		passed := evaluate(ctx, conn, cfg, logger)
		if passed {
			consecutive++
		} else {
			consecutive = 0
		}

		logger.Info("check evaluated", "name", cfg.Name, "passed", passed, "consecutive", consecutive)

		if consecutive >= cfg.SuccessThreshold {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrCanceled, ctx.Err())
		case <-time.After(cfg.Interval):
		}
	}
}

// evaluate runs a single bounded Check and applies the FailOpen policy to
// its error, if any.
func evaluate(ctx context.Context, conn connector.Connector, cfg Config, logger *slog.Logger) bool {
	checkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	passed, err := conn.Check(checkCtx)
	if err != nil {
		logger.Warn("check failed", "name", cfg.Name, "error", err, "fail_open", cfg.FailOpen)

		return cfg.FailOpen
	}

	return passed
}
