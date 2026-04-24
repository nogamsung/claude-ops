package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLimitsCmd(client *Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "limits",
		Short: "View and set task budget limits",
		Long:  "Subcommands to inspect and override daily/weekly task caps.",
	}

	cmd.AddCommand(
		newLimitsShowCmd(client),
		newLimitsSetCmd(client),
	)
	return cmd
}

func newLimitsShowCmd(client *Client) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current task limits and counters",
		Long:  "GET /modes/limits — returns daily/weekly counters, caps, and any rate-limit block.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp limitsResponse
			if err := client.get(cmd.Context(), "/modes/limits", &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}
			printLimits(p, resp)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}

func newLimitsSetCmd(client *Client) *cobra.Command {
	var daily, weekly int
	var output string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Override daily/weekly task caps",
		Long:  "PATCH /modes/limits — override daily and/or weekly caps at runtime (0 = revert to config default).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			body := limitsPatchRequest{}

			if cmd.Flags().Changed("daily") {
				d := daily
				body.DailyMax = &d
			}
			if cmd.Flags().Changed("weekly") {
				w := weekly
				body.WeeklyMax = &w
			}

			if body.DailyMax == nil && body.WeeklyMax == nil {
				return fmt.Errorf("at least one of --daily or --weekly must be specified")
			}

			var resp limitsResponse
			if err := client.patch(cmd.Context(), "/modes/limits", body, &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}
			printLimits(p, resp)
			return nil
		},
	}

	cmd.Flags().IntVar(&daily, "daily", 0, "Override daily task cap (0 = revert to config default)")
	cmd.Flags().IntVar(&weekly, "weekly", 0, "Override weekly task cap (0 = revert to config default)")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}

func printLimits(p *printer, resp limitsResponse) {
	fmt.Fprintf(p.out, "daily:  %d / %d  (%s)\n", resp.Daily.Count, resp.Daily.Max, resp.Daily.Date)
	fmt.Fprintf(p.out, "weekly: %d / %d  (%s)\n", resp.Weekly.Count, resp.Weekly.Max, resp.Weekly.Week)

	if resp.Reason != "" {
		fmt.Fprintf(p.out, "reason: %s\n", resp.Reason)
	}
	if resp.RateLimit.BlockedUntil != nil {
		fmt.Fprintf(p.out, "rate_limit_type:  %s\n", resp.RateLimit.RateLimitType)
		fmt.Fprintf(p.out, "blocked_until:    %s\n", formatTime(resp.RateLimit.BlockedUntil))
	}
}
