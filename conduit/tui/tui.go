// Package tui implements an opinionated terminal user interface conduit for
// the ore framework using Bubble Tea.
//
// Use New(mgr) to create a TUI that composes with a session.Manager. The
// TUI subscribes to the session's output stream and sends user events back
// through it. Call Run(ctx) to start the TUI and block until the user quits
// or the context is cancelled.
package tui

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/andrewhowdencom/ore/conduit"
	"github.com/andrewhowdencom/ore/loop"
	"github.com/andrewhowdencom/ore/session"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Option configures a TUI via functional options.
type Option func(*TUI)

// WithThreadID configures the TUI to attach to an existing thread instead
// of creating a new one.
func WithThreadID(id string) Option {
	return func(t *TUI) {
		t.threadID = id
	}
}

// TUI is a terminal user interface conduit. It hides all Bubble Tea internals
// from callers.
type TUI struct {
	mgr      *session.Manager
	threadID string
	eventsCh chan conduit.Event
	program  *tea.Program
}

// Descriptor enumerates the capabilities of the TUI conduit.
var Descriptor = conduit.Descriptor{
	Name:        "TUI",
	Description: "Terminal user interface via Bubble Tea",
	Capabilities: []conduit.Capability{
		conduit.CapEventSource,
		conduit.CapShowStatus,
		conduit.CapRenderTurn,
		conduit.CapRenderMarkdown,
	},
}

// New creates a new TUI conduit with the given session manager.
// The TUI does not start until Run(ctx) is called.
func New(mgr *session.Manager, opts ...Option) *TUI {
	t := &TUI{
		mgr: mgr,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// SetStatus sends a status update into the Bubble Tea message loop. The
// actual state mutation happens in model.Update.
func (t *TUI) SetStatus(ctx context.Context, status string) error {
	if t.program == nil {
		return fmt.Errorf("tui not running")
	}
	t.program.Send(statusMsg{status: status})
	return nil
}

// Run starts the TUI and blocks until the user quits or the context is
// cancelled. It satisfies conduit.Conduit.
func (t *TUI) Run(ctx context.Context) error {
	var sess session.Session
	var err error

	if t.threadID != "" {
		sess, err = t.mgr.Attach(t.threadID)
		if err != nil {
			return fmt.Errorf("attach to thread %q: %w", t.threadID, err)
		}
	} else {
		sess, err = t.mgr.Create()
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		slog.Info("thread started", "id", sess.ID())
	}

	surfEventsCh := make(chan conduit.Event, 10)
	t.eventsCh = surfEventsCh

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "> "
	// Note: bubbletea v1.3.10 does not support Shift modifier detection.
	// Alt+Enter is used as the practical alternative for newline insertion.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter"))
	ta.Focus()

	m := model{
		eventsCh: surfEventsCh,
		viewport: viewport.New(0, 0),
		textarea: ta,
		md:       newGlamourMarkdownRenderer(),
	}
	p := tea.NewProgram(&m, tea.WithAltScreen())
	t.program = p

	// Subscribe to the session's output stream.
	outputCh, err := sess.Subscribe("turn_complete")
	if err != nil {
		slog.Error("failed to subscribe to session output", "err", err)
	}

	// Goroutine to stream output events into the Bubble Tea message loop.
	if err == nil {
		go func() {
			for event := range outputCh {
				switch e := event.(type) {
				case loop.TurnCompleteEvent:
					t.program.Send(turnMsg{turn: e.Turn})
				case loop.ErrorEvent:
					// Errors are exposed via status updates rather than the
					// message loop; the application goroutine handles them.
				}
			}
		}()
	}

	// Goroutine to process user events through the session.
	go func() {
		for event := range t.eventsCh {
			switch e := event.(type) {
			case conduit.UserMessageEvent:
				if err := t.SetStatus(context.Background(), "thinking..."); err != nil {
					slog.Error("set status failed", "err", err)
				}
				if err := sess.Process(context.Background(), e); err != nil {
					slog.Error("process failed", "err", err)
					t.program.Send(clearPendingMsg{})
				}
				if err := t.SetStatus(context.Background(), ""); err != nil {
					slog.Error("set status failed", "err", err)
				}
			case conduit.InterruptEvent:
				if err := sess.Cancel(); err != nil {
					slog.Error("cancel failed", "err", err)
				}
			}
		}
	}()

	// Context cancellation handler.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			if t.program != nil {
				t.program.Send(tea.QuitMsg{})
			}
		case <-done:
		}
	}()

	_, err = t.program.Run()
	close(t.eventsCh)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
