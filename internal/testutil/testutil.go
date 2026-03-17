// Package testutil provides shared test utilities for mmi tests.
package testutil

import (
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
)

// LoadTestConfig parses config content and returns a *Config for testing.
// Fails the test on parse error.
func LoadTestConfig(t *testing.T, configContent string) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig([]byte(configContent))
	if err != nil {
		t.Fatalf("LoadTestConfig: %v", err)
	}
	return cfg
}

// MinimalTestConfig is a minimal config for testing.
const MinimalTestConfig = `
[[commands.simple]]
name = "safe"
commands = ["ls", "cat", "echo"]

[[deny.simple]]
name = "dangerous"
commands = ["rm"]
`
