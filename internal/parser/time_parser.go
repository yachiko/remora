package parser

import (
	"regexp"
	"strings"
	"time"

	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
)

// TimeParser wraps the olebedev/when library for natural language date parsing
type TimeParser struct {
	parser *when.Parser
}

// NewTimeParser creates a new time parser instance
func NewTimeParser() *TimeParser {
	w := when.New(nil)
	w.Add(en.All...)
	w.Add(common.All...)

	return &TimeParser{
		parser: w,
	}
}

// Common timezone abbreviations mapping
var timezoneAbbreviations = map[string]string{
	"EST": "America/New_York",
	"EDT": "America/New_York",
	"CST": "America/Chicago",
	"CDT": "America/Chicago",
	"MST": "America/Denver",
	"MDT": "America/Denver",
	"PST": "America/Los_Angeles",
	"PDT": "America/Los_Angeles",
	"UTC": "UTC",
	"GMT": "UTC",
}

// Regex to extract timezone from command (at the end)
var timezoneRegex = regexp.MustCompile(`(?i)\s+(EST|EDT|CST|CDT|MST|MDT|PST|PDT|UTC|GMT|America/[A-Za-z_]+|Europe/[A-Za-z_]+|Asia/[A-Za-z_]+|Pacific/[A-Za-z_]+|Atlantic/[A-Za-z_]+|Indian/[A-Za-z_]+|Australia/[A-Za-z_]+)$`)

// Regex to detect potential timezone suffix (including invalid ones)
// Matches word/path patterns at end, but will be validated against known timezones or time.LoadLocation
var potentialTimezoneRegex = regexp.MustCompile(`(?i)\s+([A-Z]{2,5}|[A-Za-z_]+/[A-Za-z_]+)$`)

// Common time units that should not be considered timezones
var timeUnits = map[string]bool{
	"day": true, "days": true,
	"week": true, "weeks": true,
	"hour": true, "hours": true,
	"minute": true, "minutes": true,
	"month": true, "months": true,
	"year": true, "years": true,
}

// Regex to detect simple duration expressions that need "in" prefix
var simpleDurationRegex = regexp.MustCompile(`^(\d+)\s+(minute|minutes|hour|hours|day|days|week|weeks|month|months|year|years)$`)

// Regex to detect combined durations like "1 week 3 days"
var combinedDurationRegex = regexp.MustCompile(`^(\d+)\s+(week|weeks|month|months)\s+(\d+)\s+(day|days|hour|hours)$`)

// ParseTime parses a natural language time expression and returns the absolute time
// It supports optional timezone suffix (defaults to UTC)
func (tp *TimeParser) ParseTime(expression string, now time.Time) (*time.Time, error) {
	// Extract timezone if present
	timezone := "UTC"
	cleanExpression := expression

	// First check for potential timezone suffix
	if potentialMatches := potentialTimezoneRegex.FindStringSubmatch(expression); len(potentialMatches) > 1 {
		tzStr := potentialMatches[1]

		// Skip if this is a common time unit (not a timezone)
		if !timeUnits[strings.ToLower(tzStr)] {
			// Check if it matches our known timezones
			if matches := timezoneRegex.FindStringSubmatch(expression); len(matches) > 1 {
				// Remove timezone from expression for parsing
				cleanExpression = timezoneRegex.ReplaceAllString(expression, "")
				cleanExpression = strings.TrimSpace(cleanExpression)

				// Map abbreviation to IANA name if needed
				if mapped, ok := timezoneAbbreviations[strings.ToUpper(tzStr)]; ok {
					timezone = mapped
				} else {
					timezone = tzStr
				}
			} else {
				// Potential timezone but not in our list - try to load it anyway
				cleanExpression = potentialTimezoneRegex.ReplaceAllString(expression, "")
				cleanExpression = strings.TrimSpace(cleanExpression)
				timezone = tzStr
			}
		}
	}

	// Load the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, NewInvalidTimezoneError(timezone)
	}

	// Use the specified timezone for the base time
	baseTime := now.In(loc)

	// Normalize expression: add "in" prefix for simple durations if not present
	normalizedExpression := cleanExpression
	if simpleDurationRegex.MatchString(cleanExpression) && !strings.HasPrefix(strings.ToLower(cleanExpression), "in ") {
		normalizedExpression = "in " + cleanExpression
	} else if combinedDurationRegex.MatchString(cleanExpression) && !strings.HasPrefix(strings.ToLower(cleanExpression), "in ") {
		normalizedExpression = "in " + cleanExpression
	}

	// Parse the time expression
	result, err := tp.parser.Parse(normalizedExpression, baseTime)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, NewNoParseError(expression)
	}

	// Convert result to UTC for storage
	parsedTime := result.Time.UTC()

	return &parsedTime, nil
}

// ParseTimeFromNow is a convenience method that parses relative to current time
func (tp *TimeParser) ParseTimeFromNow(expression string) (*time.Time, error) {
	return tp.ParseTime(expression, time.Now().UTC())
}
