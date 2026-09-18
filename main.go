package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "to-csv":
		runToCSV(os.Args[2:])
	case "to-ics":
		runToICS(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "icsconv: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `icsconv converts calendar events between iCalendar (.ics) and CSV.

Usage:
  icsconv to-csv -in events.ics [-out events.csv]
  icsconv to-ics -in events.csv [-out events.ics]
  icsconv list   -in events.ics [--json]

Paths default to stdin/stdout when omitted, so commands can be piped.
`)
}

func runToCSV(args []string) {
	fs := flag.NewFlagSet("to-csv", flag.ExitOnError)
	in := fs.String("in", "", "input .ics file (default stdin)")
	out := fs.String("out", "", "output .csv file (default stdout)")
	fs.Parse(args)

	events := mustReadICS(*in)
	mustWrite(*out, func(w io.Writer) error { return WriteCSV(w, events) })
}

func runToICS(args []string) {
	fs := flag.NewFlagSet("to-ics", flag.ExitOnError)
	in := fs.String("in", "", "input .csv file (default stdin)")
	out := fs.String("out", "", "output .ics file (default stdout)")
	fs.Parse(args)

	events := mustReadCSV(*in)
	mustWrite(*out, func(w io.Writer) error { return WriteICS(w, events) })
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	in := fs.String("in", "", "input .ics file (default stdin)")
	asJSON := fs.Bool("json", false, "print events as a JSON array instead of a table")
	fs.Parse(args)

	events := mustReadICS(*in)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(events); err != nil {
			fatal(err)
		}
		return
	}
	printTable(events)
}

func printTable(events []Event) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "UID\tSUMMARY\tSTART\tEND\tLOCATION")
	for _, e := range events {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			e.UID, e.Summary, e.Start.Format(time.RFC3339), e.End.Format(time.RFC3339), e.Location)
	}
	tw.Flush()
}

func mustReadICS(path string) []Event {
	r, closeFn := openInput(path)
	defer closeFn()
	events, err := ParseICS(r)
	if err != nil {
		fatal(fmt.Errorf("reading ics: %w", err))
	}
	return events
}

func mustReadCSV(path string) []Event {
	r, closeFn := openInput(path)
	defer closeFn()
	events, err := ReadCSV(r)
	if err != nil {
		fatal(fmt.Errorf("reading csv: %w", err))
	}
	return events
}

func mustWrite(path string, write func(io.Writer) error) {
	w, closeFn := openOutput(path)
	defer closeFn()
	if err := write(w); err != nil {
		fatal(fmt.Errorf("writing output: %w", err))
	}
}

func openInput(path string) (io.Reader, func()) {
	if path == "" || path == "-" {
		return os.Stdin, func() {}
	}
	f, err := os.Open(path)
	if err != nil {
		fatal(err)
	}
	return f, func() { f.Close() }
}

func openOutput(path string) (io.Writer, func()) {
	if path == "" || path == "-" {
		return os.Stdout, func() {}
	}
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	return f, func() { f.Close() }
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "icsconv:", err)
	os.Exit(1)
}
