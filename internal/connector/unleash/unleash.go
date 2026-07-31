// Package unleash implements a connector.Connector backed by a single
// Unleash feature flag.
package unleash

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	unleashclient "github.com/Unleash/unleash-client-go/v4"
)

// ErrSyncFailed wraps the error from the connector's most recent failed
// sync with the Unleash server.
var ErrSyncFailed = errors.New("unleash: last sync with server failed")

// Config configures a single Connector.
type Config struct {
	// Name uniquely identifies this connector instance. It namespaces the
	// underlying client's local backup file so that multiple Unleash
	// connectors never collide.
	Name string

	URL           string
	FlagName      string
	ExpectedValue bool
	Token         string

	// RefreshInterval controls how often the underlying client re-syncs
	// feature toggles from the Unleash server.
	RefreshInterval time.Duration
	// ReadyTimeout bounds how long Check waits for a successful sync
	// before reporting an error.
	ReadyTimeout time.Duration
}

// Connector checks whether an Unleash feature flag's enabled state matches
// a configured expected value.
type Connector struct {
	cfg      Config
	client   *unleashclient.Client
	listener *listener
}

// New creates a Connector and starts its background sync with the Unleash
// server. The caller must call Close when the Connector is no longer
// needed.
func New(cfg Config) (*Connector, error) {
	syncListener := newListener()

	client, err := unleashclient.NewClient(
		unleashclient.WithUrl(cfg.URL),
		unleashclient.WithAppName("k8s-chotto-matte-"+cfg.Name),
		unleashclient.WithCustomHeaders(http.Header{"Authorization": {cfg.Token}}),
		unleashclient.WithRefreshInterval(cfg.RefreshInterval),
		unleashclient.WithDisableMetrics(true),
		unleashclient.WithListener(syncListener),
	)
	if err != nil {
		return nil, fmt.Errorf("creating unleash client: %w", err)
	}

	return &Connector{cfg: cfg, client: client, listener: syncListener}, nil
}

// Check reports whether the configured flag's enabled state matches the
// configured expected value. It blocks until the client has completed at
// least one sync with the Unleash server, bounded by ctx and
// Config.ReadyTimeout.
func (c *Connector) Check(ctx context.Context) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, c.cfg.ReadyTimeout)
	defer cancel()

	readyCh := make(chan struct{})

	go func() {
		c.client.WaitForReady()
		close(readyCh)
	}()

	select {
	case <-readyCh:
	case <-checkCtx.Done():
		return false, fmt.Errorf("waiting for unleash sync: %w", checkCtx.Err())
	}

	syncErr := c.listener.lastError()
	if syncErr != nil {
		return false, fmt.Errorf("%w: %w", ErrSyncFailed, syncErr)
	}

	return c.client.IsEnabled(c.cfg.FlagName) == c.cfg.ExpectedValue, nil
}

// Close stops the connector's background sync with the Unleash server.
func (c *Connector) Close() error {
	err := c.client.Close()
	if err != nil {
		return fmt.Errorf("closing unleash client: %w", err)
	}

	return nil
}

// listener implements the unleash-client-go ErrorListener and
// RepositoryListener interfaces to track the most recent sync error. The
// client library drains these callbacks on its own goroutine, so listener
// only needs to be safe for concurrent access.
type listener struct {
	mu  sync.Mutex
	err error
}

func newListener() *listener {
	return &listener{mu: sync.Mutex{}, err: nil}
}

func (l *listener) OnError(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.err = err
}

func (l *listener) OnWarning(_ error) {}

func (l *listener) OnReady() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.err = nil
}

func (l *listener) OnUpdate() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.err = nil
}

func (l *listener) lastError() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.err
}
