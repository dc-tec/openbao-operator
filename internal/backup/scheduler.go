package backup

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	// MinScheduleInterval is the minimum allowed interval between backups.
	// Schedules more frequent than this are rejected by validation.
	MinScheduleInterval = 15 * time.Minute
	// WarnScheduleInterval is the interval below which we warn about frequent backups.
	WarnScheduleInterval = 1 * time.Hour
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

// NextSchedule calculates the next scheduled time after the given time.
func NextSchedule(expr string, from time.Time) (time.Time, error) {
	schedule, err := ParseSchedule(expr)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(from), nil
}

// GetScheduleInterval estimates the typical interval between scheduled runs.
// This is used to detect missed schedules and for validation.
func GetScheduleInterval(expr string) (time.Duration, error) {
	schedule, err := ParseSchedule(expr)
	if err != nil {
		return 0, err
	}

	// Calculate interval by checking two consecutive runs
	now := time.Now().UTC()
	next := schedule.Next(now)
	nextNext := schedule.Next(next)

	return nextNext.Sub(next), nil
}

// IsDue determines if a backup should run now.
// A backup is due if:
// - There has never been a backup (lastBackup is zero)
// - The current time is past the next scheduled time
func IsDue(expr string, lastBackup, now time.Time) (bool, error) {
	schedule, err := ParseSchedule(expr)
	if err != nil {
		return false, err
	}

	// If no previous backup, it's due immediately
	if lastBackup.IsZero() {
		return true, nil
	}

	// Calculate next scheduled time after last backup
	nextRun := schedule.Next(lastBackup)
	if now.After(nextRun) || now.Equal(nextRun) {
		return true, nil
	}

	return false, nil
}

// ValidateSchedule validates a cron expression and returns any warnings.
// It returns an error if the schedule is invalid or more frequent than MinScheduleInterval.
// It returns a warning message (non-empty string) if the schedule is more frequent than WarnScheduleInterval.
func ValidateSchedule(expr string) (warning string, err error) {
	interval, err := GetScheduleInterval(expr)
	if err != nil {
		return "", err
	}

	if interval < MinScheduleInterval {
		return "", fmt.Errorf("backup schedule interval %v is less than minimum allowed %v", interval, MinScheduleInterval)
	}

	if interval < WarnScheduleInterval {
		warning = fmt.Sprintf("backup schedule interval %v is less than recommended %v; frequent backups may impact cluster performance", interval, WarnScheduleInterval)
	}

	return warning, nil
}

// CalculateNextBackup calculates the next scheduled backup time for status reporting.
func CalculateNextBackup(expr string, lastBackup time.Time) (time.Time, error) {
	schedule, err := ParseSchedule(expr)
	if err != nil {
		return time.Time{}, err
	}

	return CalculateNextBackupAtSchedule(schedule, lastBackup, time.Now().UTC())
}

// CalculateNextBackupAt calculates the next scheduled backup time using an explicit
// "now" time, which makes it deterministic and testable.
func CalculateNextBackupAt(expr string, lastBackup, now time.Time) (time.Time, error) {
	schedule, err := ParseSchedule(expr)
	if err != nil {
		return time.Time{}, err
	}

	return CalculateNextBackupAtSchedule(schedule, lastBackup, now)
}

func CalculateNextBackupAtSchedule(schedule cron.Schedule, lastBackup, now time.Time) (time.Time, error) {
	if schedule == nil {
		return time.Time{}, fmt.Errorf("schedule must not be nil")
	}

	// If there has never been a backup/attempt, schedule the first run based on now
	// instead of running immediately. This prevents immediate execution on cluster
	// creation and respects the cron alignment.
	if lastBackup.IsZero() {
		return schedule.Next(now), nil
	}

	// Calculate next run after last backup/attempt.
	nextRun := schedule.Next(lastBackup)

	// If the next run is already in the past, treat it as missed and schedule from now.
	if nextRun.Before(now) {
		return schedule.Next(now), nil
	}

	return nextRun, nil
}
