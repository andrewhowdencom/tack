package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	oreModulePath, err := FindOreModuleRoot(".")
	require.NoError(t, err)

	tests := []struct {
		name      string
		blueprint *Blueprint
	}{
		{
			name: "http",
			blueprint: &Blueprint{
				Dist: Dist{Name: "http-agent", OutputPath: "http-agent"},
				Conduits: []ConduitConfig{
					{Module: "github.com/andrewhowdencom/ore/conduit/http"},
				},
			},
		},
		{
			name: "tui",
			blueprint: &Blueprint{
				Dist: Dist{Name: "tui-agent", OutputPath: "tui-agent"},
				Conduits: []ConduitConfig{
					{Module: "github.com/andrewhowdencom/ore/conduit/tui"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()
			outputPath := filepath.Join(outputDir, tt.blueprint.Dist.OutputPath)

			err := Build(tt.blueprint, oreModulePath, outputPath)
			require.NoError(t, err)

			info, err := os.Stat(outputPath)
			require.NoError(t, err)
			assert.False(t, info.IsDir())
		})
	}
}

func TestOreModuleName(t *testing.T) {
	t.Run("valid go.mod", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/test/module\n\ngo 1.26.2\n"), 0644))

		name, err := oreModuleName(dir)
		require.NoError(t, err)
		assert.Equal(t, "github.com/test/module", name)
	})

	t.Run("missing go.mod", func(t *testing.T) {
		_, err := oreModuleName(t.TempDir())
		require.Error(t, err)
	})

	t.Run("no module declaration", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.26.2\n"), 0644))

		_, err := oreModuleName(dir)
		require.Error(t, err)
	})
}

func TestBuild_RelativeOutputPath(t *testing.T) {
	oreModulePath, err := FindOreModuleRoot(".")
	require.NoError(t, err)

	t.Chdir(t.TempDir())

	blueprint := &Blueprint{
		Dist: Dist{Name: "rel-agent", OutputPath: "rel-agent"},
		Conduits: []ConduitConfig{
			{Module: "github.com/andrewhowdencom/ore/conduit/http"},
		},
	}

	err = Build(blueprint, oreModulePath, blueprint.Dist.OutputPath)
	require.NoError(t, err)

	info, err := os.Stat(blueprint.Dist.OutputPath)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}
