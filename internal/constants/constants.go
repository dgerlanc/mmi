// Package constants defines shared constants used across the mmi codebase.
package constants

import "os"

// File permissions
const (
	DirMode  os.FileMode = 0755
	FileMode os.FileMode = 0644
)

// Environment variables
const EnvConfigDir = "MMI_CONFIG"

// Application paths
const (
	AppName            = "mmi"
	XDGConfigSubdir    = ".config"
	ClaudeConfigDir    = ".claude"
	ClaudeSettingsFile = "settings.json"
	ConfigFileName     = "config.toml"
)
