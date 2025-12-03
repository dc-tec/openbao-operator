package backup

import (
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{
			name:    "valid daily at 3am",
			expr:    "0 3 * * *",
			wantErr: false,
		},
		{
			name:    "valid every hour",
			expr:    "0 * * * *",
			wantErr: false,
		},
		{
			name:    "valid every 15 minutes",
			expr:    "*/15 * * * *",
			wantErr: false,
		},
		{
			name:    "valid weekdays at midnight",
			expr:    "0 0 * * 1-5",
			wantErr: false,
		},
		{
			name:    "invalid - too few fields",
			expr:    "0 3 * *",
			wantErr: true,
		},
		{
			name:    "invalid - bad syntax",
			expr:    "invalid",
			wantErr: true,
		},
		{
			name:    "invalid - out of range",
			expr:    "60 3 * * *",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSchedule(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNextSchedule(t *testing.T) {
	// Use a fixed time for deterministic tests
	from := time.Date(2025, 1, 15, 2, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		expr     string
		from     time.Time
		wantTime time.Time
	}{
		{
			name:     "daily at 3am - before scheduled time",
			expr:     "0 3 * * *",
			from:     from,
			wantTime: time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
		},
		{
			name:     "daily at 3am - after scheduled time",
			expr:     "0 3 * * *",
			from:     time.Date(2025, 1, 15, 4, 0, 0, 0, time.UTC),
			wantTime: time.Date(2025, 1, 16, 3, 0, 0, 0, time.UTC),
		},
		{
			name:     "every hour on the hour",
			expr:     "0 * * * *",
			from:     from,
			wantTime: time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NextSchedule(tt.expr, tt.from)
			if err != nil {
				t.Fatalf("NextSchedule() error = %v", err)
			}

			if !got.Equal(tt.wantTime) {
				t.Errorf("NextSchedule() = %v, want %v", got, tt.wantTime)
			}
		})
	}
}

func TestGetScheduleInterval(t *testing.T) {
	tests := []struct {
		name         string
		expr         string
		wantInterval time.Duration
		wantErr      bool
	}{
		{
			name:         "daily",
			expr:         "0 3 * * *",
			wantInterval: 24 * time.Hour,
			wantErr:      false,
		},
		{
			name:         "hourly",
			expr:         "0 * * * *",
			wantInterval: 1 * time.Hour,
			wantErr:      false,
		},
		{
			name:         "every 15 minutes",
			expr:         "*/15 * * * *",
			wantInterval: 15 * time.Minute,
			wantErr:      false,
		},
		{
			name:         "every 6 hours",
			expr:         "0 */6 * * *",
			wantInterval: 6 * time.Hour,
			wantErr:      false,
		},
		{
			name:    "invalid expression",
			expr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetScheduleInterval(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetScheduleInterval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got != tt.wantInterval {
				t.Errorf("GetScheduleInterval() = %v, want %v", got, tt.wantInterval)
			}
		})
	}
}

func TestIsDue(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		lastBackup time.Time
		now        time.Time
		wantDue    bool
		wantErr    bool
	}{
		{
			name:       "no previous backup - always due",
			expr:       "0 3 * * *",
			lastBackup: time.Time{},
			now:        time.Date(2025, 1, 15, 2, 30, 0, 0, time.UTC),
			wantDue:    true,
			wantErr:    false,
		},
		{
			name:       "scheduled time passed",
			expr:       "0 3 * * *",
			lastBackup: time.Date(2025, 1, 14, 3, 0, 0, 0, time.UTC),
			now:        time.Date(2025, 1, 15, 4, 0, 0, 0, time.UTC),
			wantDue:    true,
			wantErr:    false,
		},
		{
			name:       "not yet due",
			expr:       "0 3 * * *",
			lastBackup: time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
			now:        time.Date(2025, 1, 15, 4, 0, 0, 0, time.UTC),
			wantDue:    false,
			wantErr:    false,
		},
		{
			name:       "missed schedule - last backup too old",
			expr:       "0 * * * *", // hourly
			lastBackup: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			now:        time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC), // 3 hours later
			wantDue:    true,
			wantErr:    false,
		},
		{
			name:    "invalid expression",
			expr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsDue(tt.expr, tt.lastBackup, tt.now)

			if (err != nil) != tt.wantErr {
				t.Errorf("IsDue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got != tt.wantDue {
				t.Errorf("IsDue() = %v, want %v", got, tt.wantDue)
			}
		})
	}
}

func TestValidateSchedule(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		wantWarning bool
		wantErr     bool
	}{
		{
			name:        "valid daily - no warning",
			expr:        "0 3 * * *",
			wantWarning: false,
			wantErr:     false,
		},
		{
			name:        "valid hourly - no warning",
			expr:        "0 * * * *",
			wantWarning: false,
			wantErr:     false,
		},
		{
			name:        "every 30 minutes - warning",
			expr:        "*/30 * * * *",
			wantWarning: true,
			wantErr:     false,
		},
		{
			name:        "every 10 minutes - error (too frequent)",
			expr:        "*/10 * * * *",
			wantWarning: false,
			wantErr:     true,
		},
		{
			name:        "every 5 minutes - error (too frequent)",
			expr:        "*/5 * * * *",
			wantWarning: false,
			wantErr:     true,
		},
		{
			name:    "invalid expression",
			expr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := ValidateSchedule(tt.expr)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSchedule() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			hasWarning := warning != ""
			if hasWarning != tt.wantWarning {
				t.Errorf("ValidateSchedule() warning = %v, wantWarning %v", warning, tt.wantWarning)
			}
		})
	}
}

func TestCalculateNextBackup(t *testing.T) {
	// These tests use time.Now() internally so we just verify no errors
	tests := []struct {
		name       string
		expr       string
		lastBackup time.Time
		wantErr    bool
	}{
		{
			name:       "with previous backup",
			expr:       "0 3 * * *",
			lastBackup: time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
			wantErr:    false,
		},
		{
			name:       "without previous backup",
			expr:       "0 3 * * *",
			lastBackup: time.Time{},
			wantErr:    false,
		},
		{
			name:    "invalid expression",
			expr:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateNextBackup(tt.expr, tt.lastBackup)

			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateNextBackup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
