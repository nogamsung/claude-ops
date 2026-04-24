package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newTasksCmd(client *Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Manage claude-ops tasks",
		Long:  "Subcommands to list, inspect, enqueue, and stop tasks.",
	}

	cmd.AddCommand(
		newTasksLsCmd(client),
		newTasksShowCmd(client),
		newTasksRunCmd(client),
	)

	return cmd
}

func newTasksLsCmd(client *Client) *cobra.Command {
	var status, output string

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List tasks",
		Long:  "GET /tasks — list tasks optionally filtered by status.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path := "/tasks"
			if status != "" {
				path += "?status=" + status
			}

			var resp taskListResponse
			if err := client.get(cmd.Context(), path, &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}

			headers := []string{"ID", "REPO", "ISSUE", "TYPE", "STATUS", "PR", "CREATED"}
			rows := make([][]string, len(resp.Items))
			for i, t := range resp.Items {
				pr := "-"
				if t.PRURL != "" {
					pr = t.PRURL
				}
				rows[i] = []string{
					t.ID,
					t.RepoFullName,
					fmt.Sprintf("#%d", t.IssueNumber),
					t.TaskType,
					t.Status,
					pr,
					formatTime(&t.CreatedAt),
				}
			}
			p.printTable(headers, rows)

			if resp.NextCursor != "" {
				fmt.Fprintf(p.out, "\nnext_cursor: %s\n", resp.NextCursor)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status: queued|running|done|failed|cancelled")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}

func newTasksShowCmd(client *Client) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Long:  "GET /tasks/:id — show full task details including recent events.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var resp taskDetailResponse
			if err := client.get(cmd.Context(), "/tasks/"+args[0], &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}

			fmt.Fprintf(p.out, "id:          %s\n", resp.ID)
			fmt.Fprintf(p.out, "repo:        %s\n", resp.RepoFullName)
			fmt.Fprintf(p.out, "issue:       #%d %s\n", resp.IssueNumber, resp.IssueTitle)
			fmt.Fprintf(p.out, "type:        %s\n", resp.TaskType)
			fmt.Fprintf(p.out, "status:      %s\n", resp.Status)
			fmt.Fprintf(p.out, "pr_url:      %s\n", orDash(resp.PRURL))
			fmt.Fprintf(p.out, "started_at:  %s\n", formatTime(resp.StartedAt))
			fmt.Fprintf(p.out, "finished_at: %s\n", formatTime(resp.FinishedAt))
			fmt.Fprintf(p.out, "created_at:  %s\n", formatTime(&resp.CreatedAt))

			if resp.StderrTail != "" {
				fmt.Fprintf(p.out, "\n--- stderr tail ---\n%s\n", resp.StderrTail)
			}

			if len(resp.Events) > 0 {
				fmt.Fprintln(p.out, "\n--- events ---")
				headers := []string{"KIND", "CREATED_AT", "PAYLOAD"}
				rows := make([][]string, len(resp.Events))
				for i, e := range resp.Events {
					payload := e.PayloadJSON
					if len(payload) > 60 {
						payload = payload[:60] + "..."
					}
					rows[i] = []string{e.Kind, formatTime(&e.CreatedAt), payload}
				}
				p.printTable(headers, rows)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")
	return cmd
}

func newTasksRunCmd(client *Client) *cobra.Command {
	var issueNum int
	var taskType, output string

	cmd := &cobra.Command{
		Use:   "run <owner/repo>",
		Short: "Enqueue a new task",
		Long:  "POST /tasks — enqueue a task for the given repo and issue number.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := args[0]
			if !strings.Contains(repo, "/") {
				return fmt.Errorf("repo must be in owner/repo format, got: %s", repo)
			}

			body := enqueueRequest{
				RepoFullName: repo,
				IssueNumber:  issueNum,
				IssueTitle:   taskType, // task_type is conveyed via issue_title per server convention
			}

			var resp taskResponse
			if err := client.post(cmd.Context(), "/tasks", body, &resp); err != nil {
				return err
			}

			p := newPrinter(output)
			if output == outputJSON {
				return p.printJSON(resp)
			}

			fmt.Fprintf(p.out, "enqueued task %s\n", resp.ID)
			fmt.Fprintf(p.out, "status: %s\n", resp.Status)
			return nil
		},
	}

	cmd.Flags().IntVar(&issueNum, "issue", 0, "GitHub issue number (required)")
	cmd.Flags().StringVar(&taskType, "type", "", "Task type hint: feature|security|perf")
	cmd.Flags().StringVarP(&output, "output", "o", "table", "Output format: table|json")

	if err := cmd.MarkFlagRequired("issue"); err != nil {
		// MarkFlagRequired only errors if flag name is unknown; safe to panic here.
		panic(fmt.Sprintf("flag 'issue' not found: %v", err))
	}

	return cmd
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
