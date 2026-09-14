package glance

import (
	"fmt"
	"net/http"
	"time"
)

func (a *application) handleHealthzRequest(
	w http.ResponseWriter,
	_ *http.Request,
) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	uptime := time.Since(a.CreatedAt)
	if a.CreatedAt.IsZero() || uptime < 0 {
		uptime = 0
	}

	_, _ = fmt.Fprintf(
		w,
		"Glance OK\nVersion: %s\nRevision: %s\nUptime: %s\n",
		a.Version,
		a.ShortRevision,
		formatHealthzDuration(uptime),
	)
}

func formatHealthzDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	duration = duration.Truncate(time.Second)

	days := duration / (24 * time.Hour)
	duration %= 24 * time.Hour

	hours := duration / time.Hour
	duration %= time.Hour

	minutes := duration / time.Minute
	seconds := (duration % time.Minute) / time.Second

	if days > 0 {
		return fmt.Sprintf(
			"%dd %dh %dm %ds",
			days,
			hours,
			minutes,
			seconds,
		)
	}

	if hours > 0 {
		return fmt.Sprintf(
			"%dh %dm %ds",
			hours,
			minutes,
			seconds,
		)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}

	return fmt.Sprintf("%ds", seconds)
}
