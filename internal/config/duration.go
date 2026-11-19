package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseDurationSeconds parses a positive duration expressed either as an integer number of seconds
// (e.g. "3600"), a Go duration string (e.g. "30m", "12h", "1h30m", "45s"), or day/week units
// (e.g. "7d", "1d12h", "2w") and returns whole seconds (> 0).
func ParseDurationSeconds(raw, name string) (int, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("%s must be a positive duration or number of seconds", name)
	}

	// Fast path: plain integer seconds.
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("%s must be greater than zero", name)
		}
		return n, nil
	}

	// Support optional day ("d") and week ("w") suffixes by expanding them into hours
	// before delegating to time.ParseDuration.
	expanded, err := expandDayWeekDuration(strings.ToLower(s))
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive duration (e.g. 30m, 12h, 7d) or integer seconds", name)
	}
	d, err := time.ParseDuration(expanded)
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive duration (e.g. 30m, 12h, 7d) or integer seconds", name)
	}
	if d < time.Second {
		return 0, fmt.Errorf("%s must be at least 1 second", name)
	}
	return int(d / time.Second), nil
}

func expandDayWeekDuration(s string) (string, error) {
	if !strings.ContainsAny(s, "dw") {
		return s, nil
	}
	if strings.HasPrefix(s, "-") {
		return "", fmt.Errorf("negative duration")
	}
	var out strings.Builder
	i := 0
	for i < len(s) {
		start := i
		hasDigit := false
		for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
			if s[i] >= '0' && s[i] <= '9' {
				hasDigit = true
			}
			i++
		}
		if !hasDigit || i >= len(s) {
			return "", fmt.Errorf("invalid duration")
		}
		numStr := s[start:i]
		unitStart := i
		for i < len(s) && !(s[i] >= '0' && s[i] <= '9') && s[i] != '.' {
			i++
		}
		unit := s[unitStart:i]
		switch unit {
		case "d":
			v, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return "", err
			}
			out.WriteString(strconv.FormatFloat(v*24, 'f', -1, 64))
			out.WriteByte('h')
		case "w":
			v, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return "", err
			}
			out.WriteString(strconv.FormatFloat(v*24*7, 'f', -1, 64))
			out.WriteByte('h')
		default:
			out.WriteString(numStr)
			out.WriteString(unit)
		}
	}
	return out.String(), nil
}
