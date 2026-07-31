package unleash_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pepabo/k8s-chotto-matte/internal/connector/unleash"
)

type feature struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type featureResponse struct {
	Version  int       `json:"version"`
	Features []feature `json:"features"`
}

const (
	testToken    = "test-token"
	testFlagName = "my-flag"
)

func newFeatureServer(t *testing.T, features ...feature) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/client/features" {
			http.NotFound(writer, req)

			return
		}

		if req.Header.Get("Authorization") != testToken {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(writer).Encode(featureResponse{Version: 2, Features: features})
		if err != nil {
			t.Fatalf("encoding response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func newConfig(name, baseURL string) unleash.Config {
	return unleash.Config{
		Name:            name,
		URL:             baseURL + "/api/",
		FlagName:        testFlagName,
		ExpectedValue:   true,
		Token:           testToken,
		RefreshInterval: 20 * time.Millisecond,
		ReadyTimeout:    time.Second,
	}
}

func newTestConnector(t *testing.T, cfg unleash.Config) *unleash.Connector {
	t.Helper()

	conn, err := unleash.New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() {
		closeErr := conn.Close()
		if closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	return conn
}

func TestConnector_Check_MatchesExpectedValue(t *testing.T) {
	t.Parallel()

	server := newFeatureServer(t, feature{Name: testFlagName, Enabled: true})
	conn := newTestConnector(t, newConfig("match", server.URL))

	passed, err := conn.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !passed {
		t.Errorf("Check() passed = false, want true")
	}
}

func TestConnector_Check_MismatchedValueDoesNotPass(t *testing.T) {
	t.Parallel()

	server := newFeatureServer(t, feature{Name: testFlagName, Enabled: false})
	conn := newTestConnector(t, newConfig("mismatch", server.URL))

	passed, err := conn.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if passed {
		t.Errorf("Check() passed = true, want false")
	}
}

func TestConnector_Check_TimesOutWhenServerUnreachable(t *testing.T) {
	t.Parallel()

	cfg := newConfig("unreachable", "http://127.0.0.1:1")
	cfg.ReadyTimeout = 200 * time.Millisecond

	conn := newTestConnector(t, cfg)

	_, err := conn.Check(context.Background())
	if err == nil {
		t.Fatalf("Check() error = nil, want error")
	}
}

func TestConnector_Check_ReportsSyncFailureAfterReady(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requestCount.Add(1) == 1 {
			err := json.NewEncoder(writer).Encode(featureResponse{
				Version:  2,
				Features: []feature{{Name: testFlagName, Enabled: true}},
			})
			if err != nil {
				t.Fatalf("encoding response: %v", err)
			}

			return
		}

		writer.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	cfg := newConfig("syncfail", server.URL)
	conn := newTestConnector(t, cfg)

	passed, err := conn.Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if !passed {
		t.Errorf("Check() passed = false, want true")
	}

	time.Sleep(100 * time.Millisecond)

	_, err = conn.Check(context.Background())
	if err == nil {
		t.Fatalf("Check() error = nil, want error after sync failure")
	}

	if !errors.Is(err, unleash.ErrSyncFailed) {
		t.Errorf("Check() error = %v, want wrapping %v", err, unleash.ErrSyncFailed)
	}
}
