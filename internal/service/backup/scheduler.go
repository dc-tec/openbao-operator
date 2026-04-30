package backup

import (
	"fmt"

	"github.com/robfig/cron/v3"
)

// Parser is a cron parser configured for standard 5-field cron expressions.
// It uses the standard minute, hour, day-of-month, month, day-of-week format.
var Parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// ParseSchedule parses a cron expression and returns the schedule.
func ParseSchedule(expr string) (cron.Schedule, error) {
	schedule, err := Parser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}
	return schedule, nil
}
