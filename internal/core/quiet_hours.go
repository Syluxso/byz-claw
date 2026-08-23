package core

import (
	"fmt"
	"strings"
	"time"
)

// InQuietHours reports whether now is inside [start, end) local wall times (HH:MM).
// Supports overnight windows (e.g. 23:00–07:00). Used by the scheduler (not HEARTBEAT).
func InQuietHours(now time.Time, start, end string) bool {
	if start == "" || end == "" {
		return false
	}
	sh, sm, err1 := parseHHMM(start)
	eh, em, err2 := parseHHMM(end)
	if err1 != nil || err2 != nil {
		return false
	}
	mins := now.Hour()*60 + now.Minute()
	s := sh*60 + sm
	e := eh*60 + em
	if s == e {
		return false
	}
	if s < e {
		return mins >= s && mins < e
	}
	// overnight
	return mins >= s || mins < e
}

func parseHHMM(s string) (h, m int, err error) {
	var hh, mm int
	_, err = fmt.Sscanf(strings.TrimSpace(s), "%d:%d", &hh, &mm)
	if err != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, 0, fmt.Errorf("bad time %q", s)
	}
	return hh, mm, nil
}
