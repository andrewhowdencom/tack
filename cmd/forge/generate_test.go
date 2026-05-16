package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMainGo_HTTP(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "http-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/http", Options: map[string]any{"port": "8080", "ui": true}},
		},
	}

	got, err := GenerateMainGo(blueprint)
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, `"github.com/andrewhowdencom/ore/agent"`)
	assert.Contains(t, content, `http "github.com/andrewhowdencom/ore/conduit/http"`)
	assert.Contains(t, content, `agent.New(mgr)`)
	assert.Contains(t, content, `a.Add(http.New(mgr, http.WithPort("8080"), http.WithUI()))`)
	assert.Contains(t, content, `a.Run(ctx)`)
	assert.Contains(t, content, `signal.NotifyContext`)
	assert.NotContains(t, content, `"net/http"`)
	assert.NotContains(t, content, `"flag"`)
}

func TestGenerateMainGo_TUI(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "tui-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/tui", Options: map[string]any{"thread": "abc-123"}},
		},
	}

	got, err := GenerateMainGo(blueprint)
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, `"github.com/andrewhowdencom/ore/agent"`)
	assert.Contains(t, content, `tui "github.com/andrewhowdencom/ore/conduit/tui"`)
	assert.Contains(t, content, `agent.New(mgr)`)
	assert.Contains(t, content, `a.Add(tui.New(mgr, tui.WithThreadID("abc-123")))`)
	assert.Contains(t, content, `a.Run(ctx)`)
	assert.NotContains(t, content, `"flag"`)
}

func TestGenerateMainGo_MultipleConduits(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "multi-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/http"},
			{Module: "github.com/andrewhowdencom/ore/conduit/tui"},
		},
	}

	got, err := GenerateMainGo(blueprint)
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, `http "github.com/andrewhowdencom/ore/conduit/http"`)
	assert.Contains(t, content, `tui "github.com/andrewhowdencom/ore/conduit/tui"`)
	assert.Contains(t, content, `a.Add(http.New(mgr))`)
	assert.Contains(t, content, `a.Add(tui.New(mgr))`)
}

func TestGenerateMainGo_ExternalConduit(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "ext-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/example/myconduit", Options: map[string]any{"foo": "bar"}},
		},
	}

	got, err := GenerateMainGo(blueprint)
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, `myconduit "github.com/example/myconduit"`)
	// External conduits receive no options in the first iteration.
	assert.Contains(t, content, `a.Add(myconduit.New(mgr))`)
	assert.NotContains(t, content, `foo`)
}

func TestGenerateMainGo_AliasCollision(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "collision-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/http"},
			{Module: "github.com/example/http"},
		},
	}

	got, err := GenerateMainGo(blueprint)
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, `http "github.com/andrewhowdencom/ore/conduit/http"`)
	assert.Contains(t, content, `http2 "github.com/example/http"`)
	assert.Contains(t, content, `a.Add(http.New(mgr))`)
	assert.Contains(t, content, `a.Add(http2.New(mgr))`)
}

func TestGenerateGoMod(t *testing.T) {
	blueprint := &Blueprint{
		Dist: Dist{Name: "test-agent", OutputPath: "./out"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/http"},
		},
	}

	got, err := GenerateGoMod(blueprint, "/absolute/path/to/ore")
	require.NoError(t, err)

	content := string(got)
	assert.Contains(t, content, "module test-agent")
	assert.Contains(t, content, "go 1.26.2")
	assert.Contains(t, content, "require github.com/andrewhowdencom/ore v0.0.0")
	assert.Contains(t, content, "replace github.com/andrewhowdencom/ore => /absolute/path/to/ore")
}
