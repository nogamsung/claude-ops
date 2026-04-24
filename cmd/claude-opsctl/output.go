package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
)

const outputJSON = "json"

// printer renders CLI output in table or JSON format.
type printer struct {
	format string
	out    io.Writer
}

func newPrinter(format string) *printer {
	return &printer{format: format, out: os.Stdout}
}

// printJSON serialises v as indented JSON to stdout.
func (p *printer) printJSON(v interface{}) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTable writes tab-separated rows using tabwriter.
func (p *printer) printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(p.out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush() //nolint:errcheck // tabwriter flush to stdout; error not actionable
}

// formatTime returns a human-friendly string for a nullable time.
func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
