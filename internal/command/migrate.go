// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"strings"

	"github.com/hashicorp/cli"
)

// MigrateCommand is a Command implementation that just shows help for
// the subcommands nested below it.
type MigrateCommand struct {
	Meta
}

func (c *MigrateCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *MigrateCommand) Help() string {
	helpText := `
Usage: terraform [global options] migrate <subcommand> [options] [args]

  This command has subcommands for running HCL migrations defined in JSON files.

Subcommands:
    list    List available migrations
    run     Run migrations
`
	return strings.TrimSpace(helpText)
}

func (c *MigrateCommand) Synopsis() string {
	return "Run HCL migrations defined in JSON files"
}
