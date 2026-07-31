package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pepabo/k8s-chotto-matte/internal/app"
	"github.com/pepabo/k8s-chotto-matte/internal/config"
	"github.com/pepabo/k8s-chotto-matte/internal/poller"
)

const testFlagName = "my-flag"

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

type feature struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func newFeatureServer(t *testing.T, token string, enabled bool) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != token {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(writer).Encode(map[string]any{
			"version":  2,
			"features": []feature{{Name: testFlagName, Enabled: enabled}},
		})
		if err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	err := os.WriteFile(path, []byte(contents), 0o600)
	if err != nil {
		t.Fatalf("writing config file: %v", err)
	}

	return path
}

func TestRun_AllChecksMustPass(t *testing.T) {
	serverA := newFeatureServer(t, "token-a", true)
	serverB := newFeatureServer(t, "token-b", true)

	path := writeConfig(t, `
[[checks]]
name = "a"
type = "unleash"
interval_ms = 10
success_threshold = 1
timeout_ms = 1000

  [checks.unleash]
  url = "`+serverA.URL+`/api/"
  flag_name = "my-flag"
  expected_value = true

[[checks]]
name = "b"
type = "unleash"
interval_ms = 10
success_threshold = 1
timeout_ms = 1000

  [checks.unleash]
  url = "`+serverB.URL+`/api/"
  flag_name = "my-flag"
  expected_value = true
`)
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_A", "token-a")
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_B", "token-b")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	err = app.Run(ctx, cfg, newDiscardLogger())
	if err != nil {
		t.Fatalf("app.Run() error = %v", err)
	}
}

func TestRun_ReturnsErrorWhenOneCheckNeverPasses(t *testing.T) {
	serverA := newFeatureServer(t, "token-a", true)
	serverB := newFeatureServer(t, "token-b", false) // never matches expected_value=true

	path := writeConfig(t, `
[[checks]]
name = "a"
type = "unleash"
interval_ms = 10
success_threshold = 1
timeout_ms = 200

  [checks.unleash]
  url = "`+serverA.URL+`/api/"
  flag_name = "my-flag"
  expected_value = true

[[checks]]
name = "b"
type = "unleash"
interval_ms = 10
success_threshold = 1
timeout_ms = 200
fail_open = false

  [checks.unleash]
  url = "`+serverB.URL+`/api/"
  flag_name = "my-flag"
  expected_value = true
`)
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_A", "token-a")
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_B", "token-b")

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	err = app.Run(ctx, cfg, newDiscardLogger())
	if err == nil {
		t.Fatalf("app.Run() error = nil, want error")
	}

	if !errors.Is(err, poller.ErrCanceled) {
		t.Errorf("app.Run() error = %v, want wrapping %v", err, poller.ErrCanceled)
	}
}

func TestRun_UnsupportedCheckTypeIsRejected(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Checks: []config.CheckConfig{
			{
				Name:             "bogus",
				Type:             "bogus",
				IntervalMS:       10,
				SuccessThreshold: 1,
				TimeoutMS:        10,
				FailOpen:         false,
				Unleash:          nil,
			},
		},
	}

	err := app.Run(t.Context(), cfg, newDiscardLogger())
	if err == nil {
		t.Fatalf("app.Run() error = nil, want error")
	}

	if !errors.Is(err, app.ErrUnsupportedCheckType) {
		t.Errorf("app.Run() error = %v, want wrapping %v", err, app.ErrUnsupportedCheckType)
	}
}
