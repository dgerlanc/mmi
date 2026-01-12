// Package config handles configuration loading and parsing for mmi.
package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
	"github.com/dgerlanc/mmi/internal/logger"
	"github.com/dgerlanc/mmi/patterns"
)

//go:embed config.toml
var defaultConfig []byte

// Config holds the compiled patterns from configuration.
type Config struct {
	// WrapperPatterns are safe prefixes that can wrap commands
	WrapperPatterns []patterns.Pattern
	// SafeCommands are patterns for allowed commands
	SafeCommands []patterns.Pattern
	// DenyPatterns are patterns that are always rejected (checked before approval)
	DenyPatterns []patterns.Pattern
}

var (
	// globalConfig is the loaded configuration
	globalConfig *Config
	// configInitialized tracks whether config has been loaded
	configInitialized bool
)

// GetConfigDir returns the config directory path.
// Uses MMI_CONFIG env var if set, otherwise ~/.config/mmi
func GetConfigDir() (string, error) {
	if dir := os.Getenv("MMI_CONFIG"); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mmi"), nil
}

// EnsureConfigFiles creates the config directory and writes default config file if it doesn't exist.
func EnsureConfigFiles(configDir string) error {
	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config.toml if it doesn't exist
	configPath := filepath.Join(configDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to write config.toml: %w", err)
		}
	}

	return nil
}

// parseSection parses a config section and returns compiled patterns.
// isWrapper indicates if this is a wrapper section (affects pattern generation).
func parseSection(sectionData map[string]any, isWrapper bool) ([]patterns.Pattern, error) {
	var result []patterns.Pattern

	for sectionType, value := range sectionData {
		switch sectionType {
		case "simple":
			entries := toMapSlice(value)
			for _, entry := range entries {
				name, _ := entry["name"].(string)
				cmds := toStringSlice(entry["commands"])
				for _, cmd := range cmds {
					var pattern string
					var patternName string
					if isWrapper {
						pattern = patterns.BuildWrapperPattern(cmd, nil)
						patternName = cmd
					} else {
						pattern = patterns.BuildSimplePattern(cmd)
						patternName = name
					}
					re, err := regexp.Compile(pattern)
					if err != nil {
						return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
					}
					result = append(result, patterns.Pattern{Regex: re, Name: patternName})
				}
			}

		case "command":
			entries := toMapSlice(value)
			for _, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					continue
				}
				flags := toStringSlice(entry["flags"])
				pattern := patterns.BuildWrapperPattern(cmd, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: cmd})
			}

		case "subcommand":
			entries := toMapSlice(value)
			for _, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					continue
				}
				subs := toStringSlice(entry["subcommands"])
				flags := toStringSlice(entry["flags"])
				if len(subs) == 0 {
					continue
				}
				pattern := patterns.BuildSubcommandPattern(cmd, subs, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: cmd})
			}

		case "regex":
			entries := toMapSlice(value)
			for _, entry := range entries {
				pattern, _ := entry["pattern"].(string)
				patternName, _ := entry["name"].(string)
				if pattern == "" {
					continue
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: patternName})
			}
		}
	}

	return result, nil
}

// toStringSlice converts an interface{} to []string
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// toMapSlice converts an interface{} to []map[string]any
func toMapSlice(v any) []map[string]any {
	if v == nil {
		return nil
	}
	if maps, ok := v.([]map[string]any); ok {
		return maps
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

// LoadConfig loads the config from TOML data and returns a Config.
func LoadConfig(data []byte) (*Config, error) {
	var raw map[string]map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	cfg := &Config{}

	if wrappersSection, ok := raw["wrappers"]; ok {
		wrappers, err := parseSection(wrappersSection, true)
		if err != nil {
			return nil, fmt.Errorf("failed to parse wrappers: %w", err)
		}
		cfg.WrapperPatterns = wrappers
	}

	if commandsSection, ok := raw["commands"]; ok {
		commands, err := parseSection(commandsSection, false)
		if err != nil {
			return nil, fmt.Errorf("failed to parse commands: %w", err)
		}
		cfg.SafeCommands = commands
	}

	if denySection, ok := raw["deny"]; ok {
		deny, err := parseDenySection(denySection)
		if err != nil {
			return nil, fmt.Errorf("failed to parse deny: %w", err)
		}
		cfg.DenyPatterns = deny
	}

	return cfg, nil
}

// parseDenySection parses the deny section of the config.
// Deny patterns use simple and regex subsections (no subcommand support).
func parseDenySection(sectionData map[string]any) ([]patterns.Pattern, error) {
	var result []patterns.Pattern

	for sectionType, value := range sectionData {
		switch sectionType {
		case "simple":
			// [[deny.simple]] name = "label", commands = [...]
			entries := toMapSlice(value)
			for _, entry := range entries {
				name, _ := entry["name"].(string)
				cmds := toStringSlice(entry["commands"])
				for _, cmd := range cmds {
					// For deny patterns, match the command at the start
					pattern := patterns.BuildSimplePattern(cmd)
					re, err := regexp.Compile(pattern)
					if err != nil {
						return nil, fmt.Errorf("invalid deny pattern for command %q: %w", cmd, err)
					}
					result = append(result, patterns.Pattern{Regex: re, Name: name})
				}
			}

		case "regex":
			// [[deny.regex]] pattern = "^regex", name = "desc"
			entries := toMapSlice(value)
			for _, entry := range entries {
				pattern, _ := entry["pattern"].(string)
				patternName, _ := entry["name"].(string)
				if pattern == "" {
					continue
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid deny regex pattern %q: %w", pattern, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: patternName})
			}
		}
	}

	return result, nil
}

// loadEmbeddedDefaults loads patterns from the embedded default config file.
func loadEmbeddedDefaults() *Config {
	cfg, _ := LoadConfig(defaultConfig)
	return cfg
}

// Init loads configuration from files, creating defaults if necessary.
// If loading fails, it falls back to embedded defaults.
func Init() error {
	if configInitialized {
		return nil
	}

	configDir, err := GetConfigDir()
	if err != nil {
		logger.Debug("failed to get config dir, using embedded defaults", "error", err)
		globalConfig = loadEmbeddedDefaults()
		configInitialized = true
		return err
	}

	if err := EnsureConfigFiles(configDir); err != nil {
		logger.Debug("failed to ensure config files, using embedded defaults", "error", err)
		globalConfig = loadEmbeddedDefaults()
		configInitialized = true
		return err
	}

	// Load config
	configPath := filepath.Join(configDir, "config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		logger.Debug("failed to read config file, using embedded defaults", "path", configPath, "error", err)
		globalConfig = loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to read config.toml: %w", err)
	}

	globalConfig, err = LoadConfig(configData)
	if err != nil {
		logger.Debug("failed to parse config, using embedded defaults", "error", err)
		globalConfig = loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger.Debug("config loaded successfully",
		"path", configPath,
		"wrappers", len(globalConfig.WrapperPatterns),
		"commands", len(globalConfig.SafeCommands))
	configInitialized = true
	return nil
}

// Get returns the current configuration.
// If Init has not been called, it initializes with defaults.
func Get() *Config {
	if !configInitialized {
		Init()
	}
	return globalConfig
}

// Reset resets the configuration state. Used for testing.
func Reset() {
	configInitialized = false
	globalConfig = nil
}

// GetDefaultConfig returns the embedded default configuration.
func GetDefaultConfig() []byte {
	return defaultConfig
}
