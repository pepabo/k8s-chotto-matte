// Package config loads and validates the application configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// EnvUnleashTokenPrefix is the prefix of the per-check environment variable
// that must supply an Unleash Connector's API token. The full variable name
// is EnvUnleashTokenPrefix followed by the check's upper-snake-cased name;
// see CheckConfig.UnleashTokenEnvVar. It is never read from the TOML config
// file.
//
//nolint:gosec // this is an env var name prefix, not a credential value (G101)
const EnvUnleashTokenPrefix = "CHOTTOMATTE_UNLEASH_TOKEN_"

// CheckTypeUnleash is the CheckConfig.Type value that selects the Unleash
// flag Connector.
const CheckTypeUnleash = "unleash"

// checkNamePattern restricts CheckConfig.Name so it can be safely embedded
// in an environment variable name.
var checkNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Sentinel errors returned by Load and its helpers.
var (
	ErrNoChecks             = errors.New("at least one [[checks]] entry must be configured")
	ErrCheckNameInvalid     = errors.New("checks[].name must be non-empty and match [A-Za-z0-9_-]+")
	ErrCheckNameDuplicate   = errors.New("checks[].name must be unique")
	ErrCheckTypeUnknown     = errors.New("checks[].type is unknown")
	ErrInvalidInterval      = errors.New("checks[].interval_ms must be a positive integer")
	ErrInvalidThreshold     = errors.New("checks[].success_threshold must be a positive integer")
	ErrInvalidTimeout       = errors.New("checks[].timeout_ms must be a positive integer")
	ErrUnleashConfigMissing = errors.New(`checks[].unleash must be set when type is "unleash"`)
	ErrTokenInTOML          = errors.New("checks[].unleash.token must not be set in the TOML config")
	ErrTokenEnvMissing      = errors.New("unleash token environment variable is required")
	ErrURLMissing           = errors.New("checks[].unleash.url must be set")
	ErrFlagNameMissing      = errors.New("checks[].unleash.flag_name must be set")
)

// Config is the top-level application configuration. A Pod is considered
// ready only once every configured check has passed.
type Config struct {
	Checks []CheckConfig `toml:"checks"`
}

// String implements fmt.Stringer so that printing a Config never leaks any
// check's secret token.
func (c *Config) String() string {
	checks := make([]string, len(c.Checks))
	for i := range c.Checks {
		checks[i] = c.Checks[i].String()
	}

	return fmt.Sprintf("Config{Checks:[%s]}", strings.Join(checks, ", "))
}

// LogValue implements slog.LogValuer so that logging a Config never leaks
// any check's secret token.
func (c *Config) LogValue() slog.Value {
	attrs := make([]slog.Attr, len(c.Checks))
	for i := range c.Checks {
		attrs[i] = slog.Any(c.Checks[i].Name, &c.Checks[i])
	}

	return slog.GroupValue(attrs...)
}

// CheckConfig configures a single monitored target (Connector) together
// with the common polling-loop settings applied to it.
type CheckConfig struct {
	// Name uniquely identifies this check among the configured checks. It
	// is also used to derive the environment variable name for
	// connector-specific secrets; see UnleashTokenEnvVar.
	Name string `toml:"name"`
	// Type selects which Connector implementation this check uses. Only
	// "unleash" is supported.
	Type string `toml:"type"`

	// IntervalMS is how often, in milliseconds, the connector is checked.
	IntervalMS int `toml:"interval_ms"`
	// SuccessThreshold is how many consecutive passing checks are required
	// before this check is considered passed.
	SuccessThreshold int `toml:"success_threshold"`
	// TimeoutMS bounds, in milliseconds, each individual check execution.
	TimeoutMS int `toml:"timeout_ms"`
	// FailOpen controls whether a failed check execution (e.g. a network
	// error) counts as a pass or a fail.
	FailOpen bool `toml:"fail_open"`

	// Unleash holds settings specific to the "unleash" Connector type. It
	// is set only when Type is "unleash".
	Unleash *UnleashConfig `toml:"unleash"`
}

// Interval returns the polling interval as a time.Duration.
func (c *CheckConfig) Interval() time.Duration {
	return time.Duration(c.IntervalMS) * time.Millisecond
}

// Timeout returns the per-check-execution timeout as a time.Duration.
func (c *CheckConfig) Timeout() time.Duration {
	return time.Duration(c.TimeoutMS) * time.Millisecond
}

// UnleashTokenEnvVar returns the environment variable name that must supply
// this check's Unleash API token.
func (c *CheckConfig) UnleashTokenEnvVar() string {
	return EnvUnleashTokenPrefix + strings.ToUpper(strings.ReplaceAll(c.Name, "-", "_"))
}

// String implements fmt.Stringer so that printing a CheckConfig never leaks
// the Unleash secret token.
func (c *CheckConfig) String() string {
	unleash := "nil"
	if c.Unleash != nil {
		unleash = c.Unleash.String()
	}

	return fmt.Sprintf(
		"CheckConfig{Name:%q, Type:%q, IntervalMS:%d, SuccessThreshold:%d, TimeoutMS:%d, FailOpen:%t, Unleash:%s}",
		c.Name, c.Type, c.IntervalMS, c.SuccessThreshold, c.TimeoutMS, c.FailOpen, unleash,
	)
}

// LogValue implements slog.LogValuer so that logging a CheckConfig never
// leaks the Unleash secret token.
func (c *CheckConfig) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("type", c.Type),
		slog.Int("interval_ms", c.IntervalMS),
		slog.Int("success_threshold", c.SuccessThreshold),
		slog.Int("timeout_ms", c.TimeoutMS),
		slog.Bool("fail_open", c.FailOpen),
	}

	if c.Unleash != nil {
		attrs = append(attrs, slog.Any("unleash", c.Unleash))
	}

	return slog.GroupValue(attrs...)
}

// UnleashConfig holds settings for the Unleash flag Connector.
type UnleashConfig struct {
	URL           string `toml:"url"`
	FlagName      string `toml:"flag_name"`
	ExpectedValue bool   `toml:"expected_value"`

	// Token is a secret. It must be supplied via the environment variable
	// named by CheckConfig.UnleashTokenEnvVar and must never appear in the
	// TOML file.
	Token string `toml:"token"`
}

// String implements fmt.Stringer so that printing an UnleashConfig never
// leaks the secret Token field.
func (c UnleashConfig) String() string {
	return fmt.Sprintf(
		"UnleashConfig{URL:%q, FlagName:%q, ExpectedValue:%t, Token:REDACTED}",
		c.URL, c.FlagName, c.ExpectedValue,
	)
}

// LogValue implements slog.LogValuer so that logging an UnleashConfig never
// leaks the secret Token field.
func (c UnleashConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("url", c.URL),
		slog.String("flag_name", c.FlagName),
		slog.Bool("expected_value", c.ExpectedValue),
		slog.String("token", "REDACTED"),
	)
}

// Load reads the TOML config at path, resolves each check's secrets from
// the environment, and validates the result.
func Load(path string) (*Config, error) {
	//nolint:gosec // path is an operator-supplied config location, not attacker input (G304)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config

	err = toml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	err = cfg.resolveAndValidate()
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) resolveAndValidate() error {
	if len(c.Checks) == 0 {
		return ErrNoChecks
	}

	seenNames := make(map[string]struct{}, len(c.Checks))

	for i := range c.Checks {
		err := c.Checks[i].resolveAndValidate(seenNames)
		if err != nil {
			return fmt.Errorf("checks[%d]: %w", i, err)
		}
	}

	return nil
}

func (c *CheckConfig) resolveAndValidate(seenNames map[string]struct{}) error {
	if !checkNamePattern.MatchString(c.Name) {
		return ErrCheckNameInvalid
	}

	if _, duplicate := seenNames[c.Name]; duplicate {
		return fmt.Errorf("%w: %q", ErrCheckNameDuplicate, c.Name)
	}

	seenNames[c.Name] = struct{}{}

	if c.IntervalMS <= 0 {
		return ErrInvalidInterval
	}

	if c.SuccessThreshold <= 0 {
		return ErrInvalidThreshold
	}

	if c.TimeoutMS <= 0 {
		return ErrInvalidTimeout
	}

	switch c.Type {
	case CheckTypeUnleash:
		return c.resolveAndValidateUnleash()
	default:
		return fmt.Errorf("%w: %q", ErrCheckTypeUnknown, c.Type)
	}
}

func (c *CheckConfig) resolveAndValidateUnleash() error {
	if c.Unleash == nil {
		return ErrUnleashConfigMissing
	}

	if c.Unleash.Token != "" {
		return fmt.Errorf("%w; set the %s environment variable instead", ErrTokenInTOML, c.UnleashTokenEnvVar())
	}

	envVar := c.UnleashTokenEnvVar()

	token, ok := os.LookupEnv(envVar)
	if !ok || token == "" {
		return fmt.Errorf("%w: %s", ErrTokenEnvMissing, envVar)
	}

	c.Unleash.Token = token

	if c.Unleash.URL == "" {
		return ErrURLMissing
	}

	if c.Unleash.FlagName == "" {
		return ErrFlagNameMissing
	}

	return nil
}
