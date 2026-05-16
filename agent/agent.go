// Package agent provides an orchestrator that runs multiple conduit.Conduit
// implementations concurrently against a shared session.Manager.
package agent

import (
	"context"

	"github.com/andrewhowdencom/ore/conduit"
	"github.com/andrewhowdencom/ore/session"
)

// Agent runs multiple conduits concurrently against a single session.Manager.
type Agent struct {
	mgr      *session.Manager
	conduits []conduit.Conduit
}

// New creates a new Agent with the given session manager.
func New(mgr *session.Manager) *Agent {
	return &Agent{mgr: mgr}
}

// Add registers a conduit to be run by the agent.
func (a *Agent) Add(c conduit.Conduit) {
	a.conduits = append(a.conduits, c)
}

// Run starts all registered conduits concurrently and blocks until the
// context is cancelled or any conduit exits with an error. When one conduit
// returns an error, the context is cancelled to signal the remaining
// conduits to shut down.
func (a *Agent) Run(ctx context.Context) error {
	if len(a.conduits) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(a.conduits))
	for _, c := range a.conduits {
		go func(c conduit.Conduit) {
			errCh <- c.Run(ctx)
		}(c)
	}

	var firstErr error
	for i := 0; i < len(a.conduits); i++ {
		err := <-errCh
		if firstErr == nil {
			firstErr = err
			if err != nil {
				cancel()
			}
		}
	}

	return firstErr
}
