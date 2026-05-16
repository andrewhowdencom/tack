// Package agent implements the runtime orchestrator that runs multiple I/O
// conduits concurrently against a shared session manager.
//
// Typical usage:
//
//	mgr := session.NewManager(...)
//	a := agent.New(mgr)
//	a.Add(http.New(mgr, http.WithPort("8080")))
//	a.Add(tui.New(mgr, tui.WithThreadID("abc-123")))
//	if err := a.Run(context.Background()); err != nil {
//	    log.Fatal(err)
//	}
//
// The Agent blocks until the context is cancelled or any conduit returns an
// error. When one conduit errors, the context is cancelled so remaining
// conduits shut down gracefully.
package agent
