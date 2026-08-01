package ical

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dcrespo1/kinops/internal/domain"
)

const productID = "-//KinOps//Household Calendar//EN"

func Write(writer io.Writer, feed domain.CalendarFeed) error {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:" + productID,
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:" + escapeText(feed.Name),
	}
	events := append([]domain.CalendarFeedEvent(nil), feed.Events...)
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].DueDate.Equal(events[j].DueDate) {
			return events[i].InstanceID < events[j].InstanceID
		}
		return events[i].DueDate.Before(events[j].DueDate)
	})
	for _, event := range events {
		stamp := event.UpdatedAt
		if stamp.IsZero() {
			stamp = event.DueDate
		}
		lines = append(lines,
			"BEGIN:VEVENT",
			fmt.Sprintf("UID:chore-instance-%d@kinops.local", event.InstanceID),
			"DTSTAMP:"+stamp.UTC().Format("20060102T150405Z"),
			"DTSTART;VALUE=DATE:"+event.DueDate.Format("20060102"),
			"DTEND;VALUE=DATE:"+event.DueDate.AddDate(0, 0, 1).Format("20060102"),
			"SUMMARY:"+escapeText(event.Summary),
		)
		if event.Description != "" {
			lines = append(lines, "DESCRIPTION:"+escapeText(event.Description))
		}
		if event.Category != "" {
			lines = append(lines, "CATEGORIES:"+escapeText(event.Category))
		}
		lines = append(lines, "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")
	for _, line := range lines {
		if err := writeFoldedLine(writer, line); err != nil {
			return fmt.Errorf("write calendar: %w", err)
		}
	}
	return nil
}

func escapeText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\n", "\\n",
	).Replace(value)
}

func writeFoldedLine(writer io.Writer, line string) error {
	first := true
	for {
		limit := 74
		prefix := " "
		if first {
			limit = 75
			prefix = ""
		}
		if len(line) <= limit {
			_, err := io.WriteString(writer, prefix+line+"\r\n")
			return err
		}
		boundary := limit
		for boundary > 0 && !utf8.RuneStart(line[boundary]) {
			boundary--
		}
		if _, err := io.WriteString(writer, prefix+line[:boundary]+"\r\n"); err != nil {
			return err
		}
		line = line[boundary:]
		first = false
	}
}
