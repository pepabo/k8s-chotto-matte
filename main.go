// Package main is the entry point of k8s-chotto-matte.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pepabo/k8s-chotto-matte/internal/app"
	"github.com/pepabo/k8s-chotto-matte/internal/config"
	"github.com/spf13/cobra"
)

const defaultConfigPath = "/etc/k8s-chotto-matte/config.toml"

// version is set at build time via -ldflags "-X main.version=<version>".
// It stays "dev" for local, unversioned builds.
var version = "dev"

func main() {
	err := newRootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:           "k8s-chotto-matte",
		Short:         "Delay a Pod's readiness until a monitored condition passes",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(configPath, os.Stdout)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", defaultConfigPath, "path to the TOML config file")

	return cmd
}

func run(configPath string, out io.Writer) error {
	logger := slog.New(slog.NewJSONHandler(out, nil))

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)

		return fmt.Errorf("loading config: %w", err)
	}

	logger.Info("config loaded", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = app.Run(ctx, cfg, logger)
	if err != nil {
		logger.Error("checks did not all pass", "error", err)

		return fmt.Errorf("running checks: %w", err)
	}

	logger.Info("all checks passed")

	return nil
}
