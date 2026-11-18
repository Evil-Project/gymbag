package cli

import (
	"os"
	"strings"
)

// LoadDotEnv loads key=value pairs from path into the process environment without overriding existing variables.
func LoadDotEnv(path string) {
	if path == "" {
		path = ".env"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		key, val, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		} else if idx := strings.Index(val, " #"); idx != -1 {
			val = strings.TrimSpace(val[:idx])
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
