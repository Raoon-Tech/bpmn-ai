package events

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/element"
	"github.com/Raoon-Tech/bpmn-ai/pkg/bpmn"
)

type TimerEvent struct {
	id       string
	name     string
	eventDef bpmn.EventDefinition
}

func NewTimerEvent(elem bpmn.Element) (element.Element, error) {
	return &TimerEvent{
		id:       elem.ID,
		name:     elem.Name,
		eventDef: elem.EventDefinition,
	}, nil
}

func (e *TimerEvent) ID() string {
	return e.id
}

func (e *TimerEvent) Type() bpmn.ElementType {
	return bpmn.ElementTypeTimerEvent
}

func (e *TimerEvent) Execute(_ context.Context, execCtx element.ExecutionContext) element.ExecutionResult {
	flow := execCtx.Flow()

	if e.eventDef.TimerValue == "" {
		return element.ExecutionResult{
			Action:   element.ActionRoute,
			FlowData: flow,
		}
	}

	scheduledAt := CalculateSchedule(e.eventDef.TimerType, e.eventDef.TimerValue)

	execCtx.SetVariable("timer_type", string(e.eventDef.TimerType))
	execCtx.SetVariable("timer_value", e.eventDef.TimerValue)
	if scheduledAt != nil {
		execCtx.SetVariable("timer_fire_at", scheduledAt.Format(time.RFC3339))
	}

	result := element.ExecutionResult{
		Action:   element.ActionWait,
		FlowData: flow,
	}
	if scheduledAt != nil {
		result.ContinueAt = scheduledAt
	}
	return result
}

func (e *TimerEvent) EventDefinition() bpmn.EventDefinition {
	return e.eventDef
}

func timerEventKey(elemID string) string {
	return fmt.Sprintf("timer:%s", elemID)
}

// CalculateSchedule computes the next fire time for the timer.
func CalculateSchedule(timerType bpmn.TimerType, timerValue string) *time.Time {
	switch timerType {
	case bpmn.TimerTypeDuration:
		d, err := parseISODuration(timerValue)
		if err != nil {
			return nil
		}
		t := time.Now().Add(d)
		return &t

	case bpmn.TimerTypeDate:
		t, err := time.Parse(time.RFC3339, timerValue)
		if err != nil {
			// Try ISO 8601 date formats
			for _, layout := range []string{
				"2006-01-02T15:04:05Z07:00",
				"2006-01-02T15:04:05",
				"2006-01-02",
			} {
				t, err := time.Parse(layout, timerValue)
				if err == nil {
					return &t
				}
			}
			return nil
		}
		return &t

	case bpmn.TimerTypeCycle:
		// For cron expressions, schedule the first occurrence within the next 5 years
		t, err := parseCronNext(timerValue)
		if err != nil {
			return nil
		}
		return t

	default:
		return nil
	}
}

// parseISODuration parses ISO 8601 duration strings (e.g., PT1H, P1DT30M, PT30S).
func parseISODuration(s string) (time.Duration, error) {
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("invalid ISO 8601 duration: %s (must start with P)", s)
	}

	s = strings.TrimPrefix(s, "P")
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	var total time.Duration

	// Parse date part (before T)
	datePart := s
	timePart := ""
	if idx := strings.Index(s, "T"); idx >= 0 {
		datePart = s[:idx]
		timePart = s[idx+1:]
	}

	re := regexp.MustCompile(`(\d+)([YMD])`)
	for _, m := range re.FindAllStringSubmatch(datePart, -1) {
		val, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "Y":
			total += time.Duration(val) * 365 * 24 * time.Hour
		case "M":
			total += time.Duration(val) * 30 * 24 * time.Hour
		case "D":
			total += time.Duration(val) * 24 * time.Hour
		}
	}

	re = regexp.MustCompile(`(\d+)([HMS])`)
	for _, m := range re.FindAllStringSubmatch(timePart, -1) {
		val, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "H":
			total += time.Duration(val) * time.Hour
		case "M":
			total += time.Duration(val) * time.Minute
		case "S":
			total += time.Duration(val) * time.Second
		}
	}

	return total, nil
}

// parseCronNext computes the next occurrence for a simple cron expression.
// Supports: "every X seconds/minutes/hours" and standard 5-field cron.
func parseCronNext(expr string) (*time.Time, error) {
	now := time.Now()

	// Handle simple "every X" format
	if strings.HasPrefix(expr, "every ") {
		parts := strings.Fields(expr)
		if len(parts) >= 2 {
			interval := parts[1]
			unit := "seconds"
			if len(parts) >= 3 {
				unit = parts[2]
			}

			var d time.Duration
			switch {
			case strings.HasPrefix(unit, "s"):
				d = time.Duration(parseInt(interval)) * time.Second
			case strings.HasPrefix(unit, "m"):
				d = time.Duration(parseInt(interval)) * time.Minute
			case strings.HasPrefix(unit, "h"):
				d = time.Duration(parseInt(interval)) * time.Hour
			default:
				d = time.Duration(parseInt(interval)) * time.Second
			}
			t := now.Add(d)
			return &t, nil
		}
	}

	// Try 5-field cron "min hour dom mon dow"
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		t := nextCron(now, fields)
		return &t, nil
	}

	return nil, fmt.Errorf("unrecognized cron expression: %s", expr)
}

func nextCron(now time.Time, fields []string) time.Time {
	minStr, hourStr, domStr, monStr, dowStr := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Start from the next full minute
	t := now.Truncate(time.Minute).Add(time.Minute)

	// Search up to 2 years ahead
	end := now.AddDate(2, 0, 0)

	for t.Before(end) {
		if !cronFieldMatches(monStr, int(t.Month())) {
			t = t.AddDate(0, 1, 0)
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !cronFieldMatches(domStr, t.Day()) {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !cronFieldMatches(dowStr, int(t.Weekday())) {
			t = t.Add(24 * time.Hour)
			continue
		}
		if !cronFieldMatches(hourStr, t.Hour()) {
			t = t.Add(time.Hour)
			continue
		}
		if !cronFieldMatches(minStr, t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}

	return now.Add(time.Hour)
}

func cronFieldMatches(pattern string, val int) bool {
	if pattern == "*" {
		return true
	}
	if strings.Contains(pattern, ",") {
		for _, p := range strings.Split(pattern, ",") {
			if cronFieldMatches(strings.TrimSpace(p), val) {
				return true
			}
		}
		return false
	}
	if strings.Contains(pattern, "-") {
		parts := strings.SplitN(pattern, "-", 2)
		lo, _ := strconv.Atoi(parts[0])
		hi, _ := strconv.Atoi(parts[1])
		return val >= lo && val <= hi
	}
	if strings.Contains(pattern, "/") {
		parts := strings.SplitN(pattern, "/", 2)
		step, _ := strconv.Atoi(parts[1])
		if parts[0] == "*" {
			return val%step == 0
		}
		start, _ := strconv.Atoi(parts[0])
		return val >= start && (val-start)%step == 0
	}
	n, err := strconv.Atoi(pattern)
	if err != nil {
		return false
	}
	return val == n
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
