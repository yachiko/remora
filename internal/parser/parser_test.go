package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseComment_ValidCommands(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name        string
		comment     string
		expectError bool
		description string
	}{
		{
			name:        "simple relative days",
			comment:     "remora 2 days",
			expectError: false,
			description: "Should parse '2 days' successfully",
		},
		{
			name:        "relative weeks",
			comment:     "remora 3 weeks",
			expectError: false,
			description: "Should parse '3 weeks' successfully",
		},
		{
			name:        "relative hours",
			comment:     "remora 2 hours",
			expectError: false,
			description: "Should parse '2 hours' successfully",
		},
		{
			name:        "relative minutes (30 min)",
			comment:     "remora 30 minutes",
			expectError: false,
			description: "Should parse '30 minutes' successfully",
		},
		{
			name:        "tomorrow",
			comment:     "remora tomorrow",
			expectError: false,
			description: "Should parse 'tomorrow' successfully",
		},
		{
			name:        "tomorrow with time",
			comment:     "remora tomorrow at 3pm",
			expectError: false,
			description: "Should parse 'tomorrow at 3pm' successfully",
		},
		{
			name:        "next monday",
			comment:     "remora next Monday",
			expectError: false,
			description: "Should parse 'next Monday' successfully",
		},
		{
			name:        "next monday with time",
			comment:     "remora next Monday 9am",
			expectError: false,
			description: "Should parse 'next Monday 9am' successfully",
		},
		{
			name:        "in X days format",
			comment:     "remora in 5 days",
			expectError: false,
			description: "Should parse 'in 5 days' successfully",
		},
		{
			name:        "with timezone PST",
			comment:     "remora 2 days PST",
			expectError: false,
			description: "Should parse '2 days PST' successfully",
		},
		{
			name:        "with timezone EST",
			comment:     "remora tomorrow 3pm EST",
			expectError: false,
			description: "Should parse 'tomorrow 3pm EST' successfully",
		},
		{
			name:        "case insensitive REMORA",
			comment:     "REMORA 2 days",
			expectError: false,
			description: "Should parse uppercase REMORA successfully",
		},
		{
			name:        "case insensitive Remora",
			comment:     "Remora tomorrow",
			expectError: false,
			description: "Should parse mixed case Remora successfully",
		},
		{
			name:        "embedded in text (start)",
			comment:     "remora 2 days\n\nI need to check this later.",
			expectError: false,
			description: "Should parse command at start of comment",
		},
		{
			name:        "embedded in text (middle)",
			comment:     "This is important.\n\nremora 2 days\n\nWe should revisit.",
			expectError: false,
			description: "Should parse command in middle of comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseComment(tt.comment)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none. %s", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v. %s", err, tt.description)
			}
			if !tt.expectError && result == nil {
				t.Errorf("Expected result but got nil. %s", tt.description)
			}
			if !tt.expectError && result != nil {
				if !result.RemindAt.After(time.Now().UTC()) {
					t.Errorf("RemindAt should be in the future, got %v. %s", result.RemindAt, tt.description)
				}
				duration := result.RemindAt.Sub(time.Now().UTC())
				if duration > MaxReminderDuration {
					t.Errorf("RemindAt exceeds max duration, got %v. %s", duration, tt.description)
				}
			}
		})
	}
}

func TestParseComment_InvalidCommands(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name          string
		comment       string
		expectedError ErrorType
		description   string
	}{
		{
			name:          "no command",
			comment:       "This is a regular comment with no reminder",
			expectedError: ErrorTypeNoCommand,
			description:   "Should return no command error",
		},
		{
			name:          "unparseable expression",
			comment:       "remora asap",
			expectedError: ErrorTypeNoParse,
			description:   "Should return parse error for 'asap'",
		},
		{
			name:          "unparseable vague expression",
			comment:       "remora sometime next week",
			expectedError: ErrorTypeNoParse,
			description:   "Should return parse error for vague expression",
		},
		{
			name:          "too soon (5 minutes)",
			comment:       "remora 5 minutes",
			expectedError: ErrorTypeTooSoon,
			description:   "Should return too soon error for 5 minutes",
		},
		{
			name:          "too soon (10 minutes)",
			comment:       "remora 10 minutes",
			expectedError: ErrorTypeTooSoon,
			description:   "Should return too soon error for 10 minutes",
		},
		{
			name:          "invalid timezone",
			comment:       "remora 2 days XYZ",
			expectedError: ErrorTypeInvalidTimezone,
			description:   "Should return invalid timezone error",
		},
		{
			name:          "wrong prefix (@remora)",
			comment:       "@remora 2 days",
			expectedError: ErrorTypeNoCommand,
			description:   "Should not match @remora (mentions not supported)",
		},
		{
			name:          "wrong prefix (remind me)",
			comment:       "remind me in 2 days",
			expectedError: ErrorTypeNoCommand,
			description:   "Should not match 'remind me'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseComment(tt.comment)

			if err == nil {
				t.Errorf("Expected error but got none. %s", tt.description)
				return
			}

			parseErr, ok := err.(*ParseError)
			if !ok {
				t.Errorf("Expected ParseError but got %T. %s", err, tt.description)
				return
			}

			if parseErr.Type != tt.expectedError {
				t.Errorf("Expected error type %s but got %s. %s", tt.expectedError, parseErr.Type, tt.description)
			}

			if result != nil {
				t.Errorf("Expected nil result on error but got %+v. %s", result, tt.description)
			}
		})
	}
}

func TestParseComment_BoundaryConditions(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name        string
		comment     string
		expectError bool
		errorType   ErrorType
		description string
	}{
		{
			name:        "exactly 16 minutes (safe)",
			comment:     "remora 16 minutes",
			expectError: false,
			description: "Should accept 16 minutes",
		},
		{
			name:        "14 minutes (too soon)",
			comment:     "remora 14 minutes",
			expectError: true,
			errorType:   ErrorTypeTooSoon,
			description: "Should reject 14 minutes as too soon",
		},
		{
			name:        "395 days (max boundary)",
			comment:     "remora 395 days",
			expectError: false,
			description: "Should accept exactly 395 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseComment(tt.comment)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none. %s", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v. %s", err, tt.description)
			}
			if tt.expectError && err != nil {
				parseErr, ok := err.(*ParseError)
				if !ok {
					t.Errorf("Expected ParseError but got %T. %s", err, tt.description)
				} else if parseErr.Type != tt.errorType {
					t.Errorf("Expected error type %s but got %s. %s", tt.errorType, parseErr.Type, tt.description)
				}
			}
			if !tt.expectError && result == nil {
				t.Errorf("Expected result but got nil. %s", tt.description)
			}
		})
	}
}

func TestParseComment_MultipleCommands(t *testing.T) {
	parser := NewParser()

	comment := "remora 2 days\n\nAlso, remora 1 week"

	result, err := parser.ParseComment(comment)
	if err != nil {
		t.Errorf("Expected successful parse but got error: %v", err)
		return
	}

	if !strings.Contains(result.OriginalCommand, "2 days") {
		t.Errorf("Expected first command to be parsed (2 days), got: %s", result.OriginalCommand)
	}
}

func TestHasRemoraCommand(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected bool
	}{
		{"with command", "remora 2 days", true},
		{"without command", "This is a regular comment", false},
		{"case insensitive", "REMORA tomorrow", true},
		{"embedded", "Some text\nremora 1 week\nMore text", true},
		{"wrong prefix", "@remora 2 days", false},
		{"wrong prefix 2", "remind me in 2 days", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRemoraCommand(tt.comment)
			if result != tt.expected {
				t.Errorf("HasRemoraCommand(%q) = %v, expected %v", tt.comment, result, tt.expected)
			}
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{"simple", "remora 2 days", "remora 2 days"},
		{"embedded", "Some text\nremora tomorrow\nMore text", "remora tomorrow"},
		{"no command", "This is a regular comment", ""},
		{"case insensitive", "REMORA 1 week", "REMORA 1 week"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCommand(tt.comment)
			if result != tt.expected {
				t.Errorf("ExtractCommand(%q) = %q, expected %q", tt.comment, result, tt.expected)
			}
		})
	}
}

func TestParseError_GetUserFacingMessage(t *testing.T) {
	tests := []struct {
		name     string
		error    *ParseError
		username string
		contains []string
	}{
		{
			name:     "no parse error",
			error:    NewNoParseError("remora asap"),
			username: "alice",
			contains: []string{"@alice", "couldn't parse", "Examples:", "remora 2 days"},
		},
		{
			name:     "past date error",
			error:    NewPastDateError("remora yesterday", time.Now().Add(-24*time.Hour)),
			username: "bob",
			contains: []string{"@bob", "in the past", "future date"},
		},
		{
			name:     "too soon error",
			error:    NewTooSoonError("remora 5 minutes"),
			username: "charlie",
			contains: []string{"@charlie", "too soon", "15 minutes"},
		},
		{
			name:     "too far error",
			error:    NewTooFarError("remora 2 years"),
			username: "dave",
			contains: []string{"@dave", "too far", "13 months", "395 days"},
		},
		{
			name:     "invalid timezone error",
			error:    NewInvalidTimezoneError("XYZ"),
			username: "eve",
			contains: []string{"@eve", "timezone", "EST, PST, UTC"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := tt.error.GetUserFacingMessage(tt.username)
			for _, substr := range tt.contains {
				if !strings.Contains(message, substr) {
					t.Errorf("Expected message to contain %q, got: %s", substr, message)
				}
			}
		})
	}
}

func TestReminderCommand_Fields(t *testing.T) {
	parser := NewParser()
	comment := "remora 2 days PST"

	result, err := parser.ParseComment(comment)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.OriginalCommand == "" {
		t.Error("OriginalCommand should not be empty")
	}
	if !strings.Contains(result.OriginalCommand, "remora") {
		t.Errorf("OriginalCommand should contain 'remora', got: %s", result.OriginalCommand)
	}
	if result.TimeExpression == "" {
		t.Error("TimeExpression should not be empty")
	}
	if result.RemindAt.IsZero() {
		t.Error("RemindAt should not be zero time")
	}
	if !result.RemindAt.After(time.Now()) {
		t.Error("RemindAt should be in the future")
	}
}
