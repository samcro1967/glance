package glance

import (
	"os"
	"strings"
)

const dailyDiscoveryFixtureBaseURLEnv = "GLANCE_TEST_FIXTURE_BASE_URL"

func dailyDiscoveryProviderURL(path, fallback string) string {
	base := strings.TrimSpace(os.Getenv(dailyDiscoveryFixtureBaseURLEnv))
	if base == "" {
		return fallback
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}
