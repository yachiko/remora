// Package parser provides natural language time parsing and command extraction
// for the Remora reminder service.
package parser

import (
	"fmt"
	"time"
)

// ParseError represents an error that occurred during parsing
type ParseError struct {
	Type    ErrorType
	Message string
	Details string
}

// ErrorType represents the type of parsing error
type ErrorType string

const (
	// ErrorTypeNoParse indicates the time expression could not be parsed
	ErrorTypeNoParse ErrorType = "no_parse"

	// ErrorTypePastDate indicates the parsed time is in the past
	ErrorTypePastDate ErrorType = "past_date"

	// ErrorTypeTooSoon indicates the reminder is less than 15 minutes in the future
	ErrorTypeTooSoon ErrorType = "too_soon"

	// ErrorTypeTooFar indicates the reminder is more than 395 days in the future
	ErrorTypeTooFar ErrorType = "too_far"

	// ErrorTypeNoCommand indicates no "remora" command was found in the comment
	ErrorTypeNoCommand ErrorType = "no_command"

	// ErrorTypeInvalidTimezone indicates the timezone could not be parsed
	ErrorTypeInvalidTimezone ErrorType = "invalid_timezone"
)

// Error implements the error interface
func (e *ParseError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Type, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// NewNoParseError creates an error for unparseable time expressions
func NewNoParseError(originalCommand string) *ParseError {
	return &ParseError{
		Type:    ErrorTypeNoParse,
		Message: "I couldn't parse your reminder request.",
		Details: fmt.Sprintf("Could not understand time expression in: %s", originalCommand),
	}
}

// NewPastDateError creates an error for past dates
func NewPastDateError(originalCommand string, parsedTime time.Time) *ParseError {
	return &ParseError{
		Type:    ErrorTypePastDate,
		Message: "I couldn't set your reminder because the time is in the past.",
		Details: fmt.Sprintf("You requested: %s, Parsed as: %s UTC", originalCommand, parsedTime.UTC().Format(time.RFC3339)),
	}
}

// NewTooSoonError creates an error for reminders less than 15 minutes away
func NewTooSoonError(originalCommand string) *ParseError {
	return &ParseError{
		Type:    ErrorTypeTooSoon,
		Message: "I couldn't set your reminder because it's too soon.",
		Details: fmt.Sprintf("Reminders must be at least 15 minutes in the future. You requested: %s", originalCommand),
	}
}

// NewTooFarError creates an error for reminders more than 395 days away
func NewTooFarError(originalCommand string) *ParseError {
	return &ParseError{
		Type:    ErrorTypeTooFar,
		Message: "I couldn't set your reminder because it's too far in the future.",
		Details: fmt.Sprintf("Reminders can be set up to 13 months (395 days) in advance. You requested: %s", originalCommand),
	}
}

// NewNoCommandError creates an error when no remora command is found
func NewNoCommandError() *ParseError {
	return &ParseError{
		Type:    ErrorTypeNoCommand,
		Message: "No remora command found in comment.",
		Details: "Comment must contain 'remora <time-expression>' to set a reminder",
	}
}

// NewInvalidTimezoneError creates an error for invalid timezone
func NewInvalidTimezoneError(timezone string) *ParseError {
	return &ParseError{
		Type:    ErrorTypeInvalidTimezone,
		Message: "Invalid timezone specified.",
		Details: fmt.Sprintf("Could not parse timezone: %s", timezone),
	}
}

// GetUserFacingMessage returns a user-friendly error message for posting as a comment
func (e *ParseError) GetUserFacingMessage(username string) string {
	switch e.Type {
	case ErrorTypeNoParse:
		return fmt.Sprintf("@%s I couldn't parse your reminder request.\n\nPlease use the format: `remora <time-expression>`\n\nExamples:\n- remora 2 days\n- remora tomorrow at 3pm\n- remora next Monday 9am EST\n- remora December 25th", username)

	case ErrorTypePastDate:
		return fmt.Sprintf("@%s I couldn't set your reminder because the time is in the past.\n\n%s\n\nPlease use a future date or time.\n\nExamples:\n- remora tomorrow\n- remora in 2 hours\n- remora next Monday 9am", username, e.Details)

	case ErrorTypeTooSoon:
		return fmt.Sprintf("@%s I couldn't set your reminder because it's too soon.\n\nReminders must be at least 15 minutes in the future.\n\n%s\n\nPlease use a longer timeframe:\n- remora 30 minutes\n- remora 1 hour\n- remora tomorrow", username, e.Details)

	case ErrorTypeTooFar:
		return fmt.Sprintf("@%s I couldn't set your reminder because it's too far in the future.\n\nReminders can be set up to 13 months (395 days) in advance.\n\n%s\n\nPlease use a shorter timeframe or set a reminder closer to the date.", username, e.Details)

	case ErrorTypeNoCommand:
		return fmt.Sprintf("@%s No remora command was found in your comment.\n\nPlease use the format: `remora <time-expression>`\n\nExamples:\n- remora 2 days\n- remora tomorrow at 3pm\n- remora next Monday 9am EST", username)

	case ErrorTypeInvalidTimezone:
		return fmt.Sprintf("@%s I couldn't parse the timezone in your reminder request.\n\n%s\n\nPlease use a valid timezone abbreviation (EST, PST, UTC) or IANA timezone name (America/New_York, Europe/London).\n\nExamples:\n- remora 2 days EST\n- remora tomorrow 3pm PST\n- remora next Monday 9am America/New_York", username, e.Details)

	default:
		return fmt.Sprintf("@%s I encountered an error while setting your reminder.\n\nError: %s\n\nPlease try again or contact support if the issue persists.", username, e.Message)
	}
}
