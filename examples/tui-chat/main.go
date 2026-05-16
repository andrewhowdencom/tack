// Package main provides an interactive chat REPL demonstrating the ore
// framework. It wires together the ReAct cognitive pattern, the loop.Step
// primitive for turn orchestration, and the conduit/tui package for
// terminal interaction.
//
// Usage:
//
//	go run ./examples/tui-chat
//
// Resume an existing thread:
//
//	ORE_THREAD_ID=<uuid> go run ./examples/tui-chat
//
// With persistent JSON store:
//
//	STORE_DIR=/tmp/ore-store go run ./examples/tui-chat
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/andrewhowdencom/ore/agent"
	"github.com/andrewhowdencom/ore/cognitive"
	"github.com/andrewhowdencom/ore/conduit/tui"
	"github.com/andrewhowdencom/ore/loop"
	"github.com/andrewhowdencom/ore/provider/openai"
	"github.com/andrewhowdencom/ore/session"
	"github.com/andrewhowdencom/ore/thread"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Environment configuration.
	apiKey := os.Getenv("ORE_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("ORE_API_KEY not set")
	}

	modelName := os.Getenv("ORE_MODEL")
	if modelName == "" {
		modelName = "gpt-4o"
	}

	baseURL := os.Getenv("ORE_BASE_URL")

	// Create thread store.
	var store thread.Store
	if storeDir := os.Getenv("STORE_DIR"); storeDir != "" {
		var err error
		store, err = thread.NewJSONStore(storeDir)
		if err != nil {
			return fmt.Errorf("create JSON store: %w", err)
		}
	} else {
		store = thread.NewMemoryStore()
	}

	// Build OpenAI provider.
	var opts []openai.Option
	if baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}
	prov := openai.New(apiKey, modelName, opts...)

	// Step factory for the manager.
	stepFactory := func() *loop.Step {
		return loop.New()
	}

	// Create session manager with the ReAct cognitive pattern.
	mgr := session.NewManager(store, prov, stepFactory, cognitive.NewTurnProcessor())

	// Create the agent and register the TUI conduit.
	// WithThreadID is optional; omit it to create a new thread.
	a := agent.New(mgr)
	a.Add(tui.New(mgr, tui.WithThreadID(os.Getenv("ORE_THREAD_ID"))))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run the agent. This blocks until the user quits (Ctrl+C) or the context
	// is cancelled.
	return a.Run(ctx)
}
