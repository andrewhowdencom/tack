// Package tui provides a terminal user-interface conduit for the ore
// framework. It renders conversation turns in a Bubble Tea TUI with
// Markdown formatting via glamour.
//
// Usage:
//
//	mgr := session.NewManager(...)
//	t := tui.New(mgr, tui.WithThreadID("existing-thread-id"))
//	if err := t.Run(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// WithThreadID is optional; omit it to create a new thread on startup.
package tui
