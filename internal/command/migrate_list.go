// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform/internal/migrate"
)

// MigrateListCommand implements "terraform migrate list".
type MigrateListCommand struct {
	Meta
}

func (c *MigrateListCommand) Run(args []string) int {
	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("migrate list")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	dir := "."
	if args := cmdFlags.Args(); len(args) > 0 {
		dir = args[0]
	}

	migrations, err := migrate.DiscoverMigrations(dir)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error discovering migrations: %s", err))
		return 1
	}

	if len(migrations) == 0 {
		c.Ui.Output("No migrations found.")
		return 0
	}

	for _, m := range migrations {
		if m.Description != "" {
			c.Ui.Output(fmt.Sprintf("%s - %s", m.Name, m.Description))
		} else {
			c.Ui.Output(m.Name)
		}
	}
	return 0
}

func (c *MigrateListCommand) Help() string {
	helpText := `
Usage: terraform [global options] migrate list [dir]

  Lists all available migrations found in JSON files under the given directory
  (defaults to current directory).

  Each migration file must be a JSON file with a "name", "match", and "actions" field.
`
	return strings.TrimSpace(helpText)
}

func (c *MigrateListCommand) Synopsis() string {
	return "List available migrations"
}
