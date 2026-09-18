package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var csvHeader = []string{"uid", "summary", "description", "location", "start", "end", "all_day"}

// ReadCSV expects a header row naming its columns, so column order in the
// input file doesn't matter as long as the required ones are present.
func ReadCSV(r io.Reader) ([]Event, error) {
	cr := csv.NewReader(r)
	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}

	idx := make(map[string]int, len(records[0]))
	for i, h := range records[0] {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, col := range []string{"uid", "summary", "start", "end"} {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("csv: missing required column %q", col)
		}
	}

	events := make([]Event, 0, len(records)-1)
	for _, rec := range records[1:] {
		allDay := field(rec, idx, "all_day") == "true"
		layout := "2006-01-02T15:04:05Z"
		if allDay {
			layout = "2006-01-02"
		}

		start, err := time.Parse(layout, field(rec, idx, "start"))
		if err != nil {
			return nil, fmt.Errorf("csv: bad start time %q: %w", field(rec, idx, "start"), err)
		}
		end, err := time.Parse(layout, field(rec, idx, "end"))
		if err != nil {
			return nil, fmt.Errorf("csv: bad end time %q: %w", field(rec, idx, "end"), err)
		}

		events = append(events, Event{
			UID:         field(rec, idx, "uid"),
			Summary:     field(rec, idx, "summary"),
			Description: field(rec, idx, "description"),
			Location:    field(rec, idx, "location"),
			Start:       start,
			End:         end,
			AllDay:      allDay,
		})
	}
	return events, nil
}

func field(rec []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func WriteCSV(w io.Writer, events []Event) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, e := range events {
		layout := "2006-01-02T15:04:05Z"
		if e.AllDay {
			layout = "2006-01-02"
		}
		rec := []string{
			e.UID,
			e.Summary,
			e.Description,
			e.Location,
			e.Start.UTC().Format(layout),
			e.End.UTC().Format(layout),
			strconv.FormatBool(e.AllDay),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
