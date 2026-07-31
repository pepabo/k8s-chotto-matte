package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:gosec // dummy value used only to assert it never appears in test output (G101)
const dummyToken = "dummy-super-secret-token-value"

type feature struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func newFeatureServer(t *testing.T, wantToken string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != wantToken {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		writer.Header().Set("Content-Type", "application/json")

		err := json.NewEncoder(writer).Encode(map[string]any{
			"version":  2,
			"features": []feature{{Name: "my-flag", Enabled: true}},
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

func TestRun_DoesNotLeakTokenOnSuccess(t *testing.T) {
	server := newFeatureServer(t, dummyToken)

	path := writeConfig(t, `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 10
success_threshold = 1
timeout_ms = 1000

  [checks.unleash]
  url = "`+server.URL+`/api/"
  flag_name = "my-flag"
  expected_value = true
`)
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_PRIMARY", dummyToken)

	var out bytes.Buffer

	err := run(path, &out)
	if err != nil {
		t.Fatalf("run() unexpected error: %v", err)
	}

	if strings.Contains(out.String(), dummyToken) {
		t.Errorf("run() output leaked the token; output = %s", out.String())
	}
}

func TestRun_DoesNotLeakTokenWhenTOMLContainsIt(t *testing.T) {
	path := writeConfig(t, `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 1000
success_threshold = 1
timeout_ms = 1000

  [checks.unleash]
  url = "https://unleash.example.com/api/"
  flag_name = "my-flag"
  token = "`+dummyToken+`"
`)
	t.Setenv("CHOTTOMATTE_UNLEASH_TOKEN_PRIMARY", dummyToken)

	var out bytes.Buffer

	err := run(path, &out)
	if err == nil {
		t.Fatalf("run() error = nil, want error")
	}

	if strings.Contains(err.Error(), dummyToken) {
		t.Errorf("run() error leaked the token: %v", err)
	}

	if strings.Contains(out.String(), dummyToken) {
		t.Errorf("run() output leaked the token; output = %s", out.String())
	}
}
