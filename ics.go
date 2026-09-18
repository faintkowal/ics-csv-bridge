package main

import (
	"bufio"
	"fmt"
	"io"
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
	var cur *Event
	for _, line := range lines {
		switch {
		case line == "BEGIN:VEVENT":
			cur = &Event{}
		case line == "END:VEVENT":
			if cur != nil {
				events = append(events, *cur)
				cur = nil
			}
		case cur != nil:
			name, params, value := splitProperty(line)
			applyProperty(cur, name, params, value)
		}
	}
	return events, nil
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

func applyProperty(e *Event, name string, params map[string]string, value string) {
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
