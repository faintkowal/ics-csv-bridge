# icsconv

Calendar exports come as `.ics` files, but if you want to edit a batch of
events in a spreadsheet, cross-check them against another data source, or
generate a schedule from a spreadsheet someone else maintains, you need
something that goes back and forth between iCalendar and a plain table.
That's what this is: a small command-line tool that converts between
`.ics` and CSV, in either direction, with a JSON mode for scripting.

No dependencies. Standard library only.

## Build

```
go build -o icsconv .
```

## Usage

Convert an `.ics` file to CSV:

```
icsconv to-csv -in events.ics -out events.csv
```

Convert a CSV file back to `.ics`:

```
icsconv to-ics -in events.csv -out events.ics
```

Inspect an `.ics` file without writing anything, as a table:

```
$ icsconv list -in events.ics
UID          SUMMARY        START                  END                    LOCATION
abc-123      Team standup   2026-01-15T09:00:00Z   2026-01-15T09:15:00Z   Room 4
def-456      Offsite        2026-01-20T00:00:00Z   2026-01-22T00:00:00Z
```

Or as JSON, for feeding into another tool:

```
$ icsconv list -in events.ics --json
[
  {
    "uid": "abc-123",
    "summary": "Team standup",
    "start": "2026-01-15T09:00:00Z",
    "end": "2026-01-15T09:15:00Z",
    "location": "Room 4",
    "all_day": false
  }
]
```

Any command takes `-` (or an omitted `-in`/`-out`) to mean stdin/stdout, so
these compose with shell pipelines, e.g.:

```
curl -s https://example.com/team.ics | icsconv list --json | jq '.[].summary'
```

## CSV format

The CSV has a header row and these columns: `uid`, `summary`,
`description`, `location`, `start`, `end`, `all_day`. Column order doesn't
matter as long as `uid`, `summary`, `start`, and `end` are present.
Timestamps are RFC 3339 (`2026-01-15T09:00:00Z`) unless `all_day` is
`true`, in which case they're plain dates (`2026-01-15`).

## Known limitations

This is an early skeleton. It handles single, non-recurring `VEVENT`
entries with UTC or floating times. It does not yet understand `RRULE`
recurrence or `VTIMEZONE` blocks, and ICS output isn't line-folded for
very long field values.

## License

MIT, see LICENSE.
