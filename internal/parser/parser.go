package parser

import (
	"regexp"
	"strings"
	"time"
)

// Constants for validation
const (
	// MinReminderDuration is the minimum time in the future for a reminder (15 minutes)
	MinReminderDuration = 15 * time.Minute

	// MaxReminderDuration is the maximum time in the future for a reminder (395 days / 13 months)
	MaxReminderDuration = 395 * 24 * time.Hour
)

// ReminderCommand represents a parsed reminder command
type ReminderCommand struct {
	OriginalCommand string
	TimeExpression  string
	RemindAt        time.Time
}

// Parser handles parsing of remora commands from comments
type Parser struct {
	timeParser *TimeParser
}

// NewParser creates a new parser instance
func NewParser() *Parser {
	return &Parser{
		timeParser: NewTimeParser(),
	}
}

// Regex to detect "remora" command (case-insensitive, matches first occurrence)
// Uses negative lookbehind to exclude @remora (mentions)
var remoraCommandRegex = regexp.MustCompile(`(?i)(?:^|[^@\w])remora\s+(.+?)(?:\n|$)`)

// ParseComment parses a GitHub comment body for a remora command
// Returns the parsed command or an error if parsing fails
func (p *Parser) ParseComment(commentBody string) (*ReminderCommand, error) {
	// Find the first "remora" command in the comment
	matches := remoraCommandRegex.FindStringSubmatch(commentBody)
	if len(matches) < 2 {
		return nil, NewNoCommandError()
	}

	originalCommand := strings.TrimSpace(matches[0])
	timeExpression := strings.TrimSpace(matches[1])

	// Parse the time expression
	parsedTime, err := p.timeParser.ParseTimeFromNow(timeExpression)
	if err != nil {
		// Check if it's already a ParseError
		if parseErr, ok := err.(*ParseError); ok {
			return nil, parseErr
		}
		return nil, NewNoParseError(originalCommand)
	}

	// Validate the parsed time
	if err := p.validateTime(*parsedTime, originalCommand); err != nil {
		return nil, err
	}

	return &ReminderCommand{
		OriginalCommand: originalCommand,
		TimeExpression:  timeExpression,
		RemindAt:        *parsedTime,
	}, nil
}

// validateTime validates that the parsed time meets all requirements
func (p *Parser) validateTime(parsedTime time.Time, originalCommand string) error {
	now := time.Now().UTC()

	// Check if time is in the past
	if parsedTime.Before(now) || parsedTime.Equal(now) {
		return NewPastDateError(originalCommand, parsedTime)
	}

	// Check if time is too soon (< 15 minutes)
	duration := parsedTime.Sub(now)
	if duration < MinReminderDuration {
		return NewTooSoonError(originalCommand)
	}

	// Check if time is too far (> 395 days)
	if duration > MaxReminderDuration {
		return NewTooFarError(originalCommand)
	}

	return nil
}

// ExtractCommand is a helper function that extracts just the command text from a comment
// Returns empty string if no command found
func ExtractCommand(commentBody string) string {
	matches := remoraCommandRegex.FindStringSubmatch(commentBody)
	if len(matches) < 1 {
		return ""
	}
	return strings.TrimSpace(matches[0])
}

// HasRemoraCommand checks if a comment contains a remora command
func HasRemoraCommand(commentBody string) bool {
	return remoraCommandRegex.MatchString(commentBody)
}
