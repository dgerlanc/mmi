// Package testutil provides shared test utilities for mmi tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/constants"
)

// SetupTestConfig creates a temporary config directory with test configuration.
// Returns a cleanup function that should be deferred.
func SetupTestConfig(t *testing.T, configContent string) func() {
	t.Helper()

	tmpDir := t.TempDir()
	os.Setenv(constants.EnvConfigDir, tmpDir)

	if configContent != "" {
		configPath := filepath.Join(tmpDir, constants.ConfigFileName)
		if err := os.WriteFile(configPath, []byte(configContent), constants.FileMode); err != nil {
			t.Fatal(err)
		}
	}

	config.Reset()
	config.Init()

	return func() {
		os.Unsetenv(constants.EnvConfigDir)
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
