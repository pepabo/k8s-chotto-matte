// Package app wires the configured checks into connector.Connector and
// poller.Poller instances and runs them until every one has passed.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/pepabo/k8s-chotto-matte/internal/config"
	"github.com/pepabo/k8s-chotto-matte/internal/connector"
	"github.com/pepabo/k8s-chotto-matte/internal/connector/unleash"
	"github.com/pepabo/k8s-chotto-matte/internal/poller"
)

// ErrUnsupportedCheckType is returned by Run when a check's Type has no
// matching Connector implementation.
var ErrUnsupportedCheckType = errors.New("app: unsupported check type")

// Run builds a connector.Connector for every configured check and polls all
// of them concurrently. It returns nil once every check has passed
// Config.SuccessThreshold consecutive times, and an error if ctx is
// canceled before that happens.
func Run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	conns := make([]connector.Connector, len(cfg.Checks))
	closers := make([]io.Closer, 0, len(cfg.Checks))

	defer closeAll(closers, logger)

	for checkIndex, check := range cfg.Checks {
		conn, err := newConnector(check)
		if err != nil {
			return fmt.Errorf("checks[%d] %q: %w", checkIndex, check.Name, err)
		}

		conns[checkIndex] = conn

		closers = append(closers, conn)
	}

	return runAll(ctx, cfg.Checks, conns, logger)
}

func runAll(ctx context.Context, checks []config.CheckConfig, conns []connector.Connector, logger *slog.Logger) error {
	var waitGroup sync.WaitGroup

	errs := make([]error, len(checks))

	for checkIndex, check := range checks {
		pollerCfg := poller.Config{
			Name:             check.Name,
			Interval:         check.Interval(),
			SuccessThreshold: check.SuccessThreshold,
			Timeout:          check.Timeout(),
			FailOpen:         check.FailOpen,
		}

		waitGroup.Add(1)

		go func(checkIndex int, conn connector.Connector, pollerCfg poller.Config) {
			defer waitGroup.Done()

			errs[checkIndex] = poller.Run(ctx, conn, pollerCfg, logger)
		}(checkIndex, conns[checkIndex], pollerCfg)
	}

	waitGroup.Wait()

	for checkIndex, err := range errs {
		if err != nil {
			return fmt.Errorf("checks[%d] %q: %w", checkIndex, checks[checkIndex].Name, err)
		}
	}

	return nil
}

func newConnector(check config.CheckConfig) (*unleash.Connector, error) {
	if check.Type != config.CheckTypeUnleash {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedCheckType, check.Type)
	}

	conn, err := unleash.New(unleash.Config{
		Name:            check.Name,
		URL:             check.Unleash.URL,
		FlagName:        check.Unleash.FlagName,
		ExpectedValue:   check.Unleash.ExpectedValue,
		Token:           check.Unleash.Token,
		RefreshInterval: check.Interval(),
		ReadyTimeout:    check.Timeout(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating unleash connector: %w", err)
	}

	return conn, nil
}

func closeAll(closers []io.Closer, logger *slog.Logger) {
	for _, closer := range closers {
		err := closer.Close()
		if err != nil {
			logger.Warn("failed to close connector", "error", err)
		}
	}
}
