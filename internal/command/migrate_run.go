// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform/internal/ast"
	"github.com/hashicorp/terraform/internal/migrate"
)

// MigrateRunCommand implements "terraform migrate run".
type MigrateRunCommand struct {
	Meta
}

func (c *MigrateRunCommand) Run(args []string) int {
	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("migrate run")
	var migrationsDir string
	cmdFlags.StringVar(&migrationsDir, "migrations-dir", ".", "directory containing migration JSON files")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	// Optional filter argument
	var filter string
	if args := cmdFlags.Args(); len(args) > 0 {
		filter = args[0]
	}

	// Discover and filter migrations
	migrations, err := migrate.DiscoverMigrations(migrationsDir)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error discovering migrations: %s", err))
		return 1
	}

	migrations = migrate.FilterMigrations(migrations, filter)
	if len(migrations) == 0 {
		c.Ui.Output("No matching migrations found.")
		return 0
	}

	// Parse .tf files in current directory
	tfDir := "."
	mod, err := loadModule(tfDir)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading Terraform files: %s", err))
		return 1
	}

	// Execute migrations
	for _, m := range migrations {
		c.Ui.Output(fmt.Sprintf("Running: %s", m.Name))
		if err := migrate.Execute(m, mod); err != nil {
			c.Ui.Error(fmt.Sprintf("Error running migration %s: %s", m.Name, err))
			return 1
		}
	}

	// Write back
	for filename, content := range mod.Bytes() {
		if err := os.WriteFile(filename, content, 0644); err != nil {
			c.Ui.Error(fmt.Sprintf("Error writing %s: %s", filename, err))
			return 1
		}
	}

	c.Ui.Output(fmt.Sprintf("\nApplied %d migration(s) successfully.", len(migrations)))
	return 0
}

// loadModule parses all .tf files in a directory into an ast.Module.
func loadModule(dir string) (*ast.Module, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []*ast.File
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tf") {
			continue
		}
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		f, err := ast.ParseFile(src, path, nil)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}

	return ast.NewModule(files, dir, true, nil), nil
}

func (c *MigrateRunCommand) Help() string {
	helpText := `
Usage: terraform [global options] migrate run [options] [filter]

  Runs HCL migrations defined in JSON files. The optional filter argument is a
  glob pattern matched against migration names (e.g., "v3to4/*").

Options:

  -migrations-dir=DIR  Directory containing migration JSON files (default: ".")
`
	return strings.TrimSpace(helpText)
}

func (c *MigrateRunCommand) Synopsis() string {
	return "Run HCL migrations"
}
