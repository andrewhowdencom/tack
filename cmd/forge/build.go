package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// oreModuleName extracts the Go module name from the go.mod at modulePath.
func oreModuleName(modulePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(modulePath, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.HasPrefix(trimmed, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "module ")), nil
		}
	}
	return "", fmt.Errorf("no module declaration found in %s", modulePath)
}

// Build generates a temporary Go module from blueprint, runs go mod tidy,
// and compiles a binary at outputPath using the local ore module.
// If outputPath is relative it is resolved against the current working
// directory before compilation.
func Build(blueprint *Blueprint, oreModulePath string, outputPath string) error {
	tmpDir, err := os.MkdirTemp("", "forge-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := Generate(blueprint, oreModulePath, tmpDir); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	oreModName, err := oreModuleName(oreModulePath)
	if err != nil {
		return fmt.Errorf("determine ore module name: %w", err)
	}

	// Resolve external conduit modules explicitly so that private or
	// non-proxy modules fail with clear error messages.
	for _, cfg := range blueprint.Conduits {
		if strings.HasPrefix(cfg.Module, oreModName+"/") || cfg.Module == oreModName {
			continue // local ore conduit, handled by replace directive
		}
		get := exec.Command("go", "get", cfg.Module)
		get.Dir = tmpDir
		if out, err := get.CombinedOutput(); err != nil {
			return fmt.Errorf("go get %s: %w\n%s", cfg.Module, err, out)
		}
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = tmpDir
	if out, err := tidy.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, out)
	}

	if !filepath.IsAbs(outputPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		outputPath = filepath.Join(cwd, outputPath)
	}

	build := exec.Command("go", "build", "-o", outputPath, ".")
	build.Dir = tmpDir
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %w\n%s", err, out)
	}

	return nil
}
