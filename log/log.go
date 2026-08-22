package log

import (
	"encoding/json"
	"os"
	"time"
)

// Info writes a single JSON log line to stdout (12-factor).
func Info(msg string, fields map[string]any) {
	line(msg, fields)
}

func line(msg string, fields map[string]any) {
	m := map[string]any{
		"ts":  time.Now().UTC().Format(time.RFC3339Nano),
		"msg": msg,
	}
	for k, v := range fields {
		m[k] = v
	}
	_ = json.NewEncoder(os.Stdout).Encode(m)
}
