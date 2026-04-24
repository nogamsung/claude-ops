package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newModeCmd(client *Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mode",
		Short: "Manage claude-ops operating modes",
		Long:  "Subcommands to view and toggle full mode.",
	}

	cmd.AddCommand(newModeFullCmd(client))
	return cmd
}

func newModeFullCmd(client *Client) *cobra.Command {
	var on, off bool
	var output string

	cmd := &cobra.Command{
		Use:   "full",
		Short: "View or toggle full usage mode",
		Long: `Manage full-usage mode (bypasses active-window gate).

Examples:
  claude-opsctl mode full show      # GET /modes/full
  claude-opsctl mode full --on      # POST /modes/full {"enabled":true}
  claude-opsctl mode full --off     # POST /modes/full {"enabled":false}`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// If neither --on nor --off, treat as "show"
			isShow := !on && !off
			if len(args) == 1 && args[0] == "show" {
				isShow = true
			}

			if on && off {
				return fmt.Errorf("--on and --off are mutually exclusive")
			}

			p := newPrinter(output)

			if isShow {
				var resp fullModeResponse
				if err := client.get(cmd.Context(), "/modes/full", &resp); err != nil {
					return err
				}
				if output == outputJSON {
					return p.printJSON(resp)
				}
				printFullMode(p, resp)
				return nil
			}

			body := fullModeRequest{Enabled: on}
			var resp fullModeResponse
			if err := client.post(cmd.Context(), "/modes/full", body, &resp); err != nil {
				return err
			}
			if output == outputJSON {
				return p.printJSON(resp)
			}
			printFullMode(p, resp)
			return nil
		},
	}

	cmd.Flags().BoolVar(&on, "on", false, "Enable full mode")
	cmd.Flags().BoolVar(&off, "off", false, "Disable full mode")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}

func printFullMode(p *printer, resp fullModeResponse) {
	state := "disabled"
	if resp.Enabled {
		state = "enabled"
	}
	_, _ = fmt.Fprintf(p.out, "full_mode: %s\n", state)
	_, _ = fmt.Fprintf(p.out, "since:     %s\n", formatTime(resp.Since))
}
