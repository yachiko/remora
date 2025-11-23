package parser

import (
	"testing"
	"time"
)

func TestTimeParser_ParseTime_BasicExpressions(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		expression     string
		expectError    bool
		validateResult func(t *testing.T, result *time.Time)
	}{
		{
			name:        "2 days from now",
			expression:  "2 days",
			expectError: false,
			validateResult: func(t *testing.T, result *time.Time) {
				expected := now.Add(2 * 24 * time.Hour)
				// Allow some tolerance for parsing differences
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Hour {
					t.Errorf("Expected approximately %v, got %v (diff: %v)", expected, result, diff)
				}
			},
		},
		{
			name:        "tomorrow",
			expression:  "tomorrow",
			expectError: false,
			validateResult: func(t *testing.T, result *time.Time) {
				if result.Day() != now.Add(24*time.Hour).Day() {
					t.Errorf("Expected next day, got %v", result)
				}
			},
		},
		{
			name:        "3 hours",
			expression:  "3 hours",
			expectError: false,
			validateResult: func(t *testing.T, result *time.Time) {
				expected := now.Add(3 * time.Hour)
				diff := result.Sub(expected).Abs()
				if diff > 5*time.Minute {
					t.Errorf("Expected approximately %v, got %v", expected, result)
				}
			},
		},
		{
			name:        "1 week",
			expression:  "1 week",
			expectError: false,
			validateResult: func(t *testing.T, result *time.Time) {
				expected := now.Add(7 * 24 * time.Hour)
				diff := result.Sub(expected).Abs()
				if diff > 2*time.Hour {
					t.Errorf("Expected approximately %v, got %v", expected, result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tp.ParseTime(tt.expression, now)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && result != nil && tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestTimeParser_ParseTime_WithTimezone(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		expression  string
		expectError bool
	}{
		{
			name:        "with PST timezone",
			expression:  "2 days PST",
			expectError: false,
		},
		{
			name:        "with EST timezone",
			expression:  "tomorrow EST",
			expectError: false,
		},
		{
			name:        "with UTC timezone",
			expression:  "3 hours UTC",
			expectError: false,
		},
		{
			name:        "with IANA timezone America/New_York",
			expression:  "2 days America/New_York",
			expectError: false,
		},
		{
			name:        "with IANA timezone Europe/London",
			expression:  "1 week Europe/London",
			expectError: false,
		},
		{
			name:        "with invalid timezone",
			expression:  "2 days XYZ",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tp.ParseTime(tt.expression, now)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectError && result != nil {
				// Verify result is in UTC
				if result.Location() != time.UTC {
					t.Errorf("Expected result in UTC, got %v", result.Location())
				}
				// Verify result is in the future
				if !result.After(now) {
					t.Errorf("Expected result to be after now, got %v", result)
				}
			}
		})
	}
}

func TestTimeParser_ParseTime_TimezoneAbbreviations(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC)

	abbreviations := []string{"EST", "EDT", "CST", "CDT", "MST", "MDT", "PST", "PDT", "UTC", "GMT"}

	for _, tz := range abbreviations {
		t.Run("timezone_"+tz, func(t *testing.T) {
			expression := "2 days " + tz
			result, err := tp.ParseTime(expression, now)

			if err != nil {
				t.Errorf("Expected successful parse for %s, got error: %v", tz, err)
			}
			if result == nil {
				t.Errorf("Expected result for %s, got nil", tz)
			}
			if result != nil && result.Location() != time.UTC {
				t.Errorf("Expected result in UTC, got %v", result.Location())
			}
		})
	}
}

func TestTimeParser_ParseTime_CaseInsensitive(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		expression string
	}{
		{"lowercase pst", "2 days pst"},
		{"uppercase PST", "2 days PST"},
		{"mixed case Pst", "2 days Pst"},
		{"lowercase est", "tomorrow est"},
		{"uppercase EST", "tomorrow EST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tp.ParseTime(tt.expression, now)

			if err != nil {
				t.Errorf("Expected successful parse, got error: %v", err)
			}
			if result == nil {
				t.Error("Expected result, got nil")
			}
		})
	}
}

func TestTimeParser_ParseTimeFromNow(t *testing.T) {
	tp := NewTimeParser()

	result, err := tp.ParseTimeFromNow("2 days")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Error("Expected result, got nil")
	}
	if result != nil && !result.After(time.Now()) {
		t.Error("Expected result to be in the future")
	}
}

func TestTimeParser_ParseTime_UnparseableExpression(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 12, 0, 0, 0, time.UTC)

	tests := []string{
		"asap",
		"eventually",
		"sometime",
		"xyz123",
		"",
	}

	for _, expr := range tests {
		t.Run("unparseable_"+expr, func(t *testing.T) {
			result, err := tp.ParseTime(expr, now)

			if err == nil {
				t.Errorf("Expected error for unparseable expression %q, got result: %v", expr, result)
			}
		})
	}
}

func TestTimeParser_UTCConversion(t *testing.T) {
	tp := NewTimeParser()
	now := time.Date(2025, 11, 23, 15, 0, 0, 0, time.UTC) // 3 PM UTC

	// Parse "tomorrow at 9am EST"
	result, err := tp.ParseTime("tomorrow at 9am EST", now)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Result should be in UTC
	if result.Location() != time.UTC {
		t.Errorf("Expected UTC location, got %v", result.Location())
	}

	// Verify the time makes sense
	// 9am EST = 2pm UTC (EST is UTC-5)
	if result.Hour() != 14 { // 2 PM UTC
		t.Errorf("Expected 14:00 UTC, got %02d:00 UTC", result.Hour())
	}
}
