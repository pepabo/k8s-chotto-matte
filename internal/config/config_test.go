package config_test

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/pepabo/k8s-chotto-matte/internal/config"
)

const (
	testToken       = "secret-token"
	primaryEnvVar   = "CHOTTOMATTE_UNLEASH_TOKEN_PRIMARY"
	secondaryEnvVar = "CHOTTOMATTE_UNLEASH_TOKEN_SECONDARY"
)

const validCheckTOML = `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000
fail_open = false

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "my-flag"
  expected_value = true
`

type loadTestCase struct {
	contents string
	env      map[string]string
	wantErr  error
}

func loadTestCases() map[string]loadTestCase {
	cases := map[string]loadTestCase{}

	maps.Copy(cases, validLoadTestCases())
	maps.Copy(cases, checkIdentityLoadTestCases())
	maps.Copy(cases, checkLimitLoadTestCases())
	maps.Copy(cases, unleashFieldLoadTestCases())

	return cases
}

func validLoadTestCases() map[string]loadTestCase {
	return map[string]loadTestCase{
		"valid config with token from env": {
			contents: validCheckTOML,
			env:      map[string]string{primaryEnvVar: testToken},
			wantErr:  nil,
		},
		"valid config with two checks and independent tokens": {
			contents: validCheckTOML + `
[[checks]]
name = "secondary"
type = "unleash"
interval_ms = 1000
success_threshold = 3
timeout_ms = 500
fail_open = true

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "other-flag"
  expected_value = false
`,
			env: map[string]string{
				primaryEnvVar:   testToken,
				secondaryEnvVar: "another-token",
			},
			wantErr: nil,
		},
	}
}

func checkIdentityLoadTestCases() map[string]loadTestCase {
	return map[string]loadTestCase{
		"no checks configured": {
			contents: "",
			env:      nil,
			wantErr:  config.ErrNoChecks,
		},
		"unknown check type": {
			contents: `
[[checks]]
name = "primary"
type = "bogus"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000
`,
			env:     nil,
			wantErr: config.ErrCheckTypeUnknown,
		},
		"invalid check name": {
			contents: `
[[checks]]
name = "not a valid name!"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "my-flag"
`,
			env:     nil,
			wantErr: config.ErrCheckNameInvalid,
		},
		"duplicate check names": {
			contents: validCheckTOML + `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 1000
success_threshold = 1
timeout_ms = 500

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "other-flag"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrCheckNameDuplicate,
		},
	}
}

func checkLimitLoadTestCases() map[string]loadTestCase {
	return map[string]loadTestCase{
		"non positive interval": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 0
success_threshold = 1
timeout_ms = 2000

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "my-flag"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrInvalidInterval,
		},
		"non positive success threshold": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 0
timeout_ms = 2000

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "my-flag"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrInvalidThreshold,
		},
		"non positive timeout": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 0

  [checks.unleash]
  url = "https://unleash.example.com"
  flag_name = "my-flag"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrInvalidTimeout,
		},
	}
}

func unleashFieldLoadTestCases() map[string]loadTestCase {
	return map[string]loadTestCase{
		"unleash type without unleash table": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrUnleashConfigMissing,
		},
		"missing token env var": {
			contents: validCheckTOML,
			env:      nil,
			wantErr:  config.ErrTokenEnvMissing,
		},
		"empty token env var": {
			contents: validCheckTOML,
			env:      map[string]string{primaryEnvVar: ""},
			wantErr:  config.ErrTokenEnvMissing,
		},
		"token set in toml is rejected": {
			contents: validCheckTOML + "\n[checks.unleash]\ntoken = \"leaked\"\n",
			env:      map[string]string{primaryEnvVar: testToken},
			wantErr:  config.ErrTokenInTOML,
		},
		"missing unleash url": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000

  [checks.unleash]
  flag_name = "my-flag"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrURLMissing,
		},
		"missing unleash flag name": {
			contents: `
[[checks]]
name = "primary"
type = "unleash"
interval_ms = 5000
success_threshold = 1
timeout_ms = 2000

  [checks.unleash]
  url = "https://unleash.example.com"
`,
			env:     map[string]string{primaryEnvVar: testToken},
			wantErr: config.ErrFlagNameMissing,
		},
	}
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

//nolint:paralleltest // subtests use t.Setenv, which forbids parallel execution
func TestLoad(t *testing.T) {
	for name, testCase := range loadTestCases() {
		t.Run(name, func(t *testing.T) {
			runLoadTestCase(t, testCase)
		})
	}
}

func runLoadTestCase(t *testing.T, testCase loadTestCase) {
	t.Helper()

	path := writeConfig(t, testCase.contents)

	for k, v := range testCase.env {
		t.Setenv(k, v)
	}

	cfg, err := config.Load(path)
	if testCase.wantErr != nil {
		if err == nil {
			t.Fatalf("Load() error = nil, want error wrapping %v", testCase.wantErr)
		}

		return
	}

	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	for _, check := range cfg.Checks {
		wantToken := testCase.env[check.UnleashTokenEnvVar()]
		if check.Unleash.Token != wantToken {
			t.Errorf("cfg.Checks[%q].Unleash.Token = %q, want %q", check.Name, check.Unleash.Token, wantToken)
		}
	}
}
