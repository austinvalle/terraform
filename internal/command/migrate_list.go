// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: BUSL-1.1

package command

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform/internal/backend/backendrun"
	"github.com/hashicorp/terraform/internal/command/arguments"
	"github.com/hashicorp/terraform/internal/command/views"
	"github.com/hashicorp/terraform/internal/migrate"
	"github.com/hashicorp/terraform/internal/terraform"
	"github.com/hashicorp/terraform/internal/tfdiags"
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

	// Load backend (following import command as example :P)
	var diags tfdiags.Diagnostics
	b, backendDiags := c.backend(dir, arguments.ViewHuman)
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Seems reasonable to assume this should be a local operation only for the moment
	local, ok := b.(backendrun.Local)
	if !ok {
		c.Ui.Error(ErrUnsupportedLocalOp)
		return 1
	}

	// Build the operation
	var err error
	opReq := c.Operation(b, arguments.ViewHuman)
	opReq.ConfigDir = dir
	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}
	opReq.Hooks = []terraform.Hook{c.uiHook()}

	// TODO: skipping variables from command line args for now
	// {
	// 	// Collect variable value and add them to the operation request
	// 	var varDiags tfdiags.Diagnostics
	// 	opReq.Variables, varDiags = parsedArgs.Vars.CollectValues(func(filename string, src []byte) {
	// 		opReq.ConfigLoader.Parser().ForceFileSource(filename, src)
	// 	})
	// 	diags = diags.Append(varDiags)

	// 	if varDiags.HasErrors() {
	// 		c.showDiagnostics(diags)
	// 		return 1
	// 	}

	// 	c.VariableValues = opReq.Variables
	// }

	opReq.View = views.NewOperation(arguments.ViewHuman, c.RunningInAutomation, c.View)

	// Get the context
	lr, _, ctxDiags := local.LocalRun(opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	migrations := make([]*migrate.Migration, 0)
	providerMigrations, moreDiags := lr.Core.CodeMigrations(lr.Config, lr.InputState)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}
	migrations = append(migrations, providerMigrations...)

	jsonMigrations, err := migrate.DiscoverMigrations(dir)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error discovering migrations: %s", err))
		return 1
	}
	migrations = append(migrations, jsonMigrations...)

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
