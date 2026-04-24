package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newHealthCmd(client *Client) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check the claude-ops API health",
		Long:  "GET /healthz — returns server status, full-mode flag, and next tick time.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp healthResponse
			if err := client.get(cmd.Context(), "/healthz", &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}

			tickAt := "-"
			if resp.TickAt != nil {
				tickAt = formatTime(resp.TickAt)
			}
			_, _ = fmt.Fprintf(p.out, "status:    %s\n", resp.Status)
			_, _ = fmt.Fprintf(p.out, "full_mode: %v\n", resp.FullMode)
			_, _ = fmt.Fprintf(p.out, "tick_at:   %s\n", tickAt)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}
