package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/main.go.tmpl
var mainGoTmpl string

//go:embed templates/go.mod.tmpl
var goModTmpl string

// conduitData holds per-conduit template parameters.
type conduitData struct {
	ModulePath  string
	ImportAlias string
	NewCall     string
}

// deriveImportAlias returns a unique import alias for the given module path.
// If the last path segment is already in use, a numeric suffix is appended.
func deriveImportAlias(modulePath string, used map[string]bool) string {
	parts := strings.Split(modulePath, "/")
	base := parts[len(parts)-1]

	if !used[base] {
		used[base] = true
		return base
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// formatNewCall generates the Go expression for creating a conduit,
// including typed functional options for built-in ore conduits.
func formatNewCall(alias, modulePath string, options map[string]any) string {
	var opts []string

	switch modulePath {
	case "github.com/andrewhowdencom/ore/conduit/http":
		if port, ok := options["port"]; ok {
			opts = append(opts, fmt.Sprintf(`%s.WithPort("%v")`, alias, port))
		}
		if ui, ok := options["ui"]; ok && ui == true {
			opts = append(opts, fmt.Sprintf(`%s.WithUI()`, alias))
		}
	case "github.com/andrewhowdencom/ore/conduit/tui":
		if thread, ok := options["thread"]; ok {
			opts = append(opts, fmt.Sprintf(`%s.WithThreadID("%v")`, alias, thread))
		}
	}

	if len(opts) > 0 {
		return fmt.Sprintf(`%s.New(mgr, %s)`, alias, strings.Join(opts, ", "))
	}
	return fmt.Sprintf(`%s.New(mgr)`, alias)
}

// prepareConduitData converts blueprint conduit configs into template data.
func prepareConduitData(blueprint *Blueprint) []conduitData {
	used := make(map[string]bool)
	var result []conduitData

	for _, cfg := range blueprint.Conduits {
		alias := deriveImportAlias(cfg.Module, used)
		newCall := formatNewCall(alias, cfg.Module, cfg.Options)
		result = append(result, conduitData{
			ModulePath:  cfg.Module,
			ImportAlias: alias,
			NewCall:     newCall,
		})
	}

	return result
}

// GenerateMainGo produces a compilable main.go for the conduit specified
// in blueprint.
func GenerateMainGo(blueprint *Blueprint) ([]byte, error) {
	tmpl, err := template.New("main").Parse(mainGoTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse main.go template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		Conduits []conduitData
	}{
		Conduits: prepareConduitData(blueprint),
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute main.go template: %w", err)
	}

	// Verify generated code is valid Go syntax.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "main.go", buf.Bytes(), parser.AllErrors); err != nil {
		return nil, fmt.Errorf("generated main.go is invalid Go: %w", err)
	}

	return buf.Bytes(), nil
}

// Generate writes main.go and go.mod into targetDir.
func Generate(blueprint *Blueprint, oreModulePath string, targetDir string) error {
	mainGo, err := GenerateMainGo(blueprint)
	if err != nil {
		return fmt.Errorf("generate main.go: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "main.go"), mainGo, 0644); err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	goMod, err := GenerateGoMod(blueprint, oreModulePath)
	if err != nil {
		return fmt.Errorf("generate go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "go.mod"), goMod, 0644); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}

	return nil
}

// GenerateGoMod produces a go.mod that depends on the local ore module via
// a replace directive.
func GenerateGoMod(blueprint *Blueprint, oreModulePath string) ([]byte, error) {
	tmpl, err := template.New("gomod").Parse(goModTmpl)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		ModuleName    string
		OreModulePath string
	}{
		ModuleName:    blueprint.Dist.Name,
		OreModulePath: oreModulePath,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute go.mod template: %w", err)
	}

	return buf.Bytes(), nil
}
