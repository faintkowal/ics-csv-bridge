package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// unescapeText and escapeText only cover the four escapes RFC 5545 defines
// for TEXT values. Good enough for real-world calendar exports.
var unescaper = strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`)
var escaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `,`, `\,`, `;`, `\;`)

// ParseICS reads a VCALENDAR and returns every VEVENT it contains.
// It unfolds continuation lines first, since a single property can be
// wrapped across several physical lines in the source file.
func ParseICS(r io.Reader) ([]Event, error) {
	lines, err := unfoldLines(r)
	if err != nil {
		return nil, err
	}

	var events []Event
	var cur *pendingEvent
	for _, line := range lines {
		switch {
		case line == "BEGIN:VEVENT":
			cur = &pendingEvent{}
		case line == "END:VEVENT":
			if cur != nil {
				if cur.rrule == "" {
					events = append(events, cur.Event)
				} else {
					events = append(events, expandRRule(cur.Event, cur.rrule)...)
				}
				cur = nil
			}
		case cur != nil:
			name, params, value := splitProperty(line)
			applyProperty(cur, name, params, value)
		}
	}
	return events, nil
}

// pendingEvent holds an Event plus the raw RRULE text while a VEVENT is
// still being read. RRULE isn't part of Event because a recurring VEVENT
// expands into several plain Events before ParseICS returns.
type pendingEvent struct {
	Event
	rrule string
}

func unfoldLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		l := strings.TrimRight(scanner.Text(), "\r")
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += l[1:]
		} else {
			lines = append(lines, l)
		}
	}
	return lines, scanner.Err()
}

// splitProperty turns "DTSTART;VALUE=DATE:20260115" into
// name="DTSTART", params={"VALUE":"DATE"}, value="20260115".
func splitProperty(line string) (name string, params map[string]string, value string) {
	idx := strings.IndexByte(line, ':')
	if idx == -1 {
		return strings.ToUpper(line), nil, ""
	}
	head, value := line[:idx], line[idx+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(parts[0])
	if len(parts) > 1 {
		params = make(map[string]string, len(parts)-1)
		for _, p := range parts[1:] {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 {
				params[strings.ToUpper(kv[0])] = kv[1]
			}
		}
	}
	return name, params, value
}

func applyProperty(e *pendingEvent, name string, params map[string]string, value string) {
	switch name {
	case "UID":
		e.UID = unescaper.Replace(value)
	case "SUMMARY":
		e.Summary = unescaper.Replace(value)
	case "DESCRIPTION":
		e.Description = unescaper.Replace(value)
	case "LOCATION":
		e.Location = unescaper.Replace(value)
	case "DTSTART":
		if t, allDay, err := parseICSTime(value, params); err == nil {
			e.Start = t
			e.AllDay = allDay
		}
	case "DTEND":
		if t, _, err := parseICSTime(value, params); err == nil {
			e.End = t
		}
	case "RRULE":
		e.rrule = value
	}
}

func parseICSTime(value string, params map[string]string) (t time.Time, allDay bool, err error) {
	if params["VALUE"] == "DATE" || len(value) == 8 {
		t, err = time.Parse("20060102", value)
		return t, true, err
	}
	if strings.HasSuffix(value, "Z") {
		t, err = time.Parse("20060102T150405Z", value)
		return t, false, err
	}
	// No timezone info in the property, and no VTIMEZONE support yet:
	// treat it as UTC rather than silently guessing a local offset.
	t, err = time.Parse("20060102T150405", value)
	return t.UTC(), false, err
}

// maxRecurrences caps how many occurrences an RRULE expands into, so a
// rule with neither COUNT nor UNTIL (an open-ended recurrence) can't blow
// up memory on a malformed or intentionally huge input.
const maxRecurrences = 500

type rrule struct {
	freq     string
	interval int
	count    int
	until    time.Time
	byDay    []time.Weekday
}

var weekdayCodes = map[string]time.Weekday{
	"SU": time.Sunday,
	"MO": time.Monday,
	"TU": time.Tuesday,
	"WE": time.Wednesday,
	"TH": time.Thursday,
	"FR": time.Friday,
	"SA": time.Saturday,
}

// expandRRule turns a recurring VEVENT into the individual Events it
// represents. On an RRULE this parser doesn't understand, it falls back
// to the single occurrence from DTSTART/DTEND rather than dropping it.
func expandRRule(base Event, value string) []Event {
	rule, err := parseRRule(value)
	if err != nil {
		return []Event{base}
	}
	return expandRecurrence(base, rule)
}

func parseRRule(value string) (rrule, error) {
	r := rrule{interval: 1}
	for _, part := range strings.Split(value, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.ToUpper(kv[0]), kv[1]
		switch key {
		case "FREQ":
			r.freq = strings.ToUpper(val)
		case "INTERVAL":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				r.interval = n
			}
		case "COUNT":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				r.count = n
			}
		case "UNTIL":
			if t, err := parseUntil(val); err == nil {
				r.until = t
			}
		case "BYDAY":
			r.byDay = parseByDay(val)
		}
	}
	switch r.freq {
	case "DAILY", "WEEKLY", "MONTHLY", "YEARLY":
	default:
		return rrule{}, fmt.Errorf("ics: unsupported RRULE FREQ %q", r.freq)
	}
	return r, nil
}

func parseUntil(value string) (time.Time, error) {
	if len(value) == 8 {
		return time.Parse("20060102", value)
	}
	if strings.HasSuffix(value, "Z") {
		return time.Parse("20060102T150405Z", value)
	}
	return time.Parse("20060102T150405", value)
}

// parseByDay reads a BYDAY value like "MO,WE,FR". Ordinal prefixes such
// as the "1" in "1MO" are for MONTHLY/YEARLY rules, which this parser
// doesn't apply BYDAY to, so they're stripped and ignored.
func parseByDay(value string) []time.Weekday {
	var days []time.Weekday
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if len(part) < 2 {
			continue
		}
		code := part[len(part)-2:]
		if wd, ok := weekdayCodes[code]; ok {
			days = append(days, wd)
		}
	}
	return days
}

func expandRecurrence(base Event, rule rrule) []Event {
	duration := base.End.Sub(base.Start)
	limit := rule.count
	if limit <= 0 || limit > maxRecurrences {
		limit = maxRecurrences
	}

	var starts []time.Time
	if rule.freq == "WEEKLY" && len(rule.byDay) > 0 {
		starts = weeklyByDayStarts(base.Start, rule, limit)
	} else {
		starts = simpleStarts(base.Start, rule, limit)
	}

	events := make([]Event, 0, len(starts))
	for _, s := range starts {
		ev := base
		ev.Start = s
		ev.End = s.Add(duration)
		events = append(events, ev)
	}
	return events
}

func simpleStarts(start time.Time, rule rrule, limit int) []time.Time {
	var out []time.Time
	cur := start
	for len(out) < limit {
		if !rule.until.IsZero() && cur.After(rule.until) {
			break
		}
		out = append(out, cur)
		if rule.count > 0 && len(out) >= rule.count {
			break
		}
		cur = advanceByFreq(cur, rule.freq, rule.interval)
	}
	return out
}

func advanceByFreq(t time.Time, freq string, interval int) time.Time {
	switch freq {
	case "DAILY":
		return t.AddDate(0, 0, interval)
	case "WEEKLY":
		return t.AddDate(0, 0, 7*interval)
	case "MONTHLY":
		return t.AddDate(0, interval, 0)
	case "YEARLY":
		return t.AddDate(interval, 0, 0)
	default:
		return t.AddDate(0, 0, interval)
	}
}

// weeklyByDayStarts expands a WEEKLY RRULE with a BYDAY list, walking one
// interval-week window at a time and emitting the matching weekdays in
// each window in order.
func weeklyByDayStarts(start time.Time, rule rrule, limit int) []time.Time {
	var out []time.Time
	weekStart := start.AddDate(0, 0, -weekdayOffset(start.Weekday()))
	for {
		for _, wd := range sortedByWeekStart(rule.byDay) {
			day := weekStart.AddDate(0, 0, weekdayOffset(wd))
			occurrence := time.Date(day.Year(), day.Month(), day.Day(),
				start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), start.Location())
			if occurrence.Before(start) {
				continue
			}
			if !rule.until.IsZero() && occurrence.After(rule.until) {
				return out
			}
			out = append(out, occurrence)
			if rule.count > 0 && len(out) >= rule.count {
				return out
			}
			if len(out) >= limit {
				return out
			}
		}
		weekStart = weekStart.AddDate(0, 0, 7*rule.interval)
	}
}

// weekdayOffset returns how many days wd falls after Monday, treating the
// week as starting on Monday (RFC 5545's default WKST).
func weekdayOffset(wd time.Weekday) int {
	return (int(wd) - int(time.Monday) + 7) % 7
}

func sortedByWeekStart(days []time.Weekday) []time.Weekday {
	out := make([]time.Weekday, len(days))
	copy(out, days)
	sort.Slice(out, func(i, j int) bool {
		return weekdayOffset(out[i]) < weekdayOffset(out[j])
	})
	return out
}

// WriteICS renders events as a minimal but valid VCALENDAR.
func WriteICS(w io.Writer, events []Event) error {
	bw := bufio.NewWriter(w)
	writeLine(bw, "BEGIN:VCALENDAR")
	writeLine(bw, "VERSION:2.0")
	writeLine(bw, "PRODID:-//icsconv//EN")
	for _, e := range events {
		writeLine(bw, "BEGIN:VEVENT")
		writeLine(bw, "UID:"+escaper.Replace(e.UID))
		if e.AllDay {
			writeLine(bw, "DTSTART;VALUE=DATE:"+e.Start.Format("20060102"))
			writeLine(bw, "DTEND;VALUE=DATE:"+e.End.Format("20060102"))
		} else {
			writeLine(bw, "DTSTART:"+e.Start.UTC().Format("20060102T150405Z"))
			writeLine(bw, "DTEND:"+e.End.UTC().Format("20060102T150405Z"))
		}
		writeLine(bw, "SUMMARY:"+escaper.Replace(e.Summary))
		if e.Description != "" {
			writeLine(bw, "DESCRIPTION:"+escaper.Replace(e.Description))
		}
		if e.Location != "" {
			writeLine(bw, "LOCATION:"+escaper.Replace(e.Location))
		}
		writeLine(bw, "END:VEVENT")
	}
	writeLine(bw, "END:VCALENDAR")
	return bw.Flush()
}

func writeLine(w *bufio.Writer, s string) {
	fmt.Fprintf(w, "%s\r\n", s)
}
