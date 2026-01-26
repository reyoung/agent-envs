package jobqueue

import (
	"fmt"
	"strings"
)

// New creates a JobQueue implementation based on the DSN schema.
func New(dsn string) (JobQueue, error) {
	scheme, hasScheme := extractScheme(dsn)
	switch {
	case !hasScheme:
		return newRedisJobQueue(dsn)
	case scheme == "redis" || scheme == "rediss":
		return newRedisJobQueue(dsn)
	default:
		return nil, fmt.Errorf("unsupported job queue scheme %q", scheme)
	}
}

func extractScheme(dsn string) (string, bool) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return "", false
	}

	i := strings.Index(dsn, "://")
	if i == -1 {
		return "", false
	}

	return strings.ToLower(strings.TrimSpace(dsn[:i])), true
}
