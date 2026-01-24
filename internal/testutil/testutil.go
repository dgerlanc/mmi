// Package testutil provides shared test utilities for mmi tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
)

// SetupTestConfig creates a temporary config directory with test configuration.
// Returns a cleanup function that should be deferred.
func SetupTestConfig(t *testing.T, configContent string) func() {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv("MMI_CONFIG", tmpDir)

	if configContent != "" {
		configPath := filepath.Join(tmpDir, "config.toml")
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	config.Reset()
	config.Init()

	return func() {
		os.Unsetenv("MMI_CONFIG")
		config.Reset()
	}
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
