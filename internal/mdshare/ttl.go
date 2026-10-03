package mdshare

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const DefaultTTL = 24 * time.Hour

// IsNever reports whether s asks for a share that never expires.
func IsNever(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "never", "keep", "forever":
		return true
	}
	return false
}

// ParseExpiry parses a --ttl value: a duration, or "never".
func ParseExpiry(s string) (Expiry, error) {
	if IsNever(s) {
		return Expiry{Keep: true}, nil
	}
	d, err := ParseTTL(s)
	return Expiry{TTL: d}, err
}

// ParseTTL parses a duration such as "90m", "24h" or "7d".
func ParseTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))

	var d time.Duration
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid ttl %q", s)
		}
		d = time.Duration(n * float64(24*time.Hour))
	} else {
		var err error
		d, err = time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid ttl %q (use e.g. 1h, 24h, 7d or never)", s)
		}
	}

	if d < time.Second {
		return 0, fmt.Errorf("invalid ttl %q: must be at least 1s", s)
	}
	return d, nil
}

// FormatTTL renders a duration the way ParseTTL accepts it.
func FormatTTL(d time.Duration) string {
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", d/(24*time.Hour))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", d/time.Hour)
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}
