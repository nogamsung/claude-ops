// Package main is the entry point for the claude-opsctl CLI.
//
// claude-opsctl is a thin HTTP wrapper CLI for the claude-ops local API
// (default: http://127.0.0.1:8787). Override the endpoint via CLAUDE_OPS_URL.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	client := newClient()

	root := &cobra.Command{
		Use:   "claude-opsctl",
		Short: "CLI client for the claude-ops HTTP API",
		Long: `claude-opsctl wraps the claude-ops REST API (http://127.0.0.1:8787 by default).

Set CLAUDE_OPS_URL to override the endpoint.

All commands support --output table|json (-o).`,
		SilenceUsage: true,
	}

	root.AddCommand(
		newHealthCmd(client),
		newTasksCmd(client),
		newModeCmd(client),
		newLimitsCmd(client),
	)

	return root
}
