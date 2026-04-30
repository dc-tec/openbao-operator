package backup

import (
	"testing"
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
