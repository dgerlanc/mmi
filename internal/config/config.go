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
	"github.com/dgerlanc/mmi/internal/patterns"
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

// sectionOptions controls how a config section is parsed.
type sectionOptions struct {
	name            string // Section name for error messages: "wrappers", "commands", "deny"
	isWrapper       bool   // Affects pattern generation for simple commands
	allowSubcommand bool   // Whether to parse subcommand entries
	allowCommand    bool   // Whether to parse command entries
}

// parseSection parses a config section and returns compiled patterns.
// The options parameter controls which entry types are allowed and how patterns are generated.
func parseSection(sectionData map[string]any, opts sectionOptions) ([]patterns.Pattern, error) {
	var result []patterns.Pattern

	for sectionType, value := range sectionData {
		switch sectionType {
		case "simple":
			entries := toMapSlice(value)
			for i, entry := range entries {
				name, _ := entry["name"].(string)
				cmds := toStringSlice(entry["commands"])
				if len(cmds) == 0 {
					if name != "" {
						return nil, fmt.Errorf("%s.simple[%d] %q: \"commands\" field is required and must not be empty", opts.name, i, name)
					}
					return nil, fmt.Errorf("%s.simple[%d]: \"commands\" field is required and must not be empty", opts.name, i)
				}
				for _, cmd := range cmds {
					var pattern string
					var patternName string
					if opts.isWrapper {
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
					result = append(result, patterns.Pattern{Regex: re, Name: patternName, Type: "simple", Pattern: pattern})
				}
			}

		case "command":
			if !opts.allowCommand {
				continue
			}
			entries := toMapSlice(value)
			for i, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					return nil, fmt.Errorf("%s.command[%d]: \"command\" field is required and must not be empty", opts.name, i)
				}
				flags := toStringSlice(entry["flags"])
				pattern := patterns.BuildWrapperPattern(cmd, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: cmd, Type: "command", Pattern: pattern})
			}

		case "subcommand":
			if !opts.allowSubcommand {
				continue
			}
			entries := toMapSlice(value)
			for i, entry := range entries {
				cmd, _ := entry["command"].(string)
				if cmd == "" {
					return nil, fmt.Errorf("%s.subcommand[%d]: \"command\" field is required and must not be empty", opts.name, i)
				}
				subs := toStringSlice(entry["subcommands"])
				flags := toStringSlice(entry["flags"])
				if len(subs) == 0 {
					return nil, fmt.Errorf("%s.subcommand[%d] %q: \"subcommands\" field is required and must not be empty", opts.name, i, cmd)
				}
				pattern := patterns.BuildSubcommandPattern(cmd, subs, flags)
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for command %q: %w", cmd, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: cmd, Type: "subcommand", Pattern: pattern})
			}

		case "regex":
			entries := toMapSlice(value)
			for i, entry := range entries {
				pattern, _ := entry["pattern"].(string)
				patternName, _ := entry["name"].(string)
				if pattern == "" {
					if patternName != "" {
						return nil, fmt.Errorf("%s.regex[%d] %q: \"pattern\" field is required and must not be empty", opts.name, i, patternName)
					}
					return nil, fmt.Errorf("%s.regex[%d]: \"pattern\" field is required and must not be empty", opts.name, i)
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
				}
				result = append(result, patterns.Pattern{Regex: re, Name: patternName, Type: "regex", Pattern: pattern})
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
// configDir is optional and only needed when the config uses include directives.
func LoadConfig(data []byte) (*Config, error) {
	return LoadConfigWithDir(data, "")
}

// LoadConfigWithDir loads the config from TOML data with a base directory for includes.
func LoadConfigWithDir(data []byte, configDir string) (*Config, error) {
	return loadConfigWithIncludes(data, configDir, make(map[string]bool))
}

// loadConfigWithIncludes loads config with include support and cycle detection.
func loadConfigWithIncludes(data []byte, configDir string, visited map[string]bool) (*Config, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	cfg := &Config{}

	// Process includes first
	if includeVal, ok := raw["include"]; ok {
		includes := toStringSlice(includeVal)
		for _, include := range includes {
			if configDir == "" {
				logger.Debug("include directive ignored (no config directory)", "include", include)
				continue
			}

			includePath := filepath.Join(configDir, include)

			// Check for cycles
			absPath, err := filepath.Abs(includePath)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve include path %q: %w", include, err)
			}
			if visited[absPath] {
				return nil, fmt.Errorf("circular include detected: %s", include)
			}
			visited[absPath] = true

			// Load included file
			includeData, err := os.ReadFile(includePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read include file %q: %w", include, err)
			}

			logger.Debug("loading include", "path", includePath)
			includeCfg, err := loadConfigWithIncludes(includeData, configDir, visited)
			if err != nil {
				return nil, fmt.Errorf("failed to parse include file %q: %w", include, err)
			}

			// Merge included config
			cfg.WrapperPatterns = append(cfg.WrapperPatterns, includeCfg.WrapperPatterns...)
			cfg.SafeCommands = append(cfg.SafeCommands, includeCfg.SafeCommands...)
			cfg.DenyPatterns = append(cfg.DenyPatterns, includeCfg.DenyPatterns...)
		}
	}

	// Parse sections from this file
	if wrappersSection, ok := raw["wrappers"].(map[string]any); ok {
		wrappers, err := parseSection(wrappersSection, sectionOptions{
			name:         "wrappers",
			isWrapper:    true,
			allowCommand: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse wrappers: %w", err)
		}
		cfg.WrapperPatterns = append(cfg.WrapperPatterns, wrappers...)
	}

	if commandsSection, ok := raw["commands"].(map[string]any); ok {
		commands, err := parseSection(commandsSection, sectionOptions{
			name:            "commands",
			allowSubcommand: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse commands: %w", err)
		}
		cfg.SafeCommands = append(cfg.SafeCommands, commands...)
	}

	if denySection, ok := raw["deny"].(map[string]any); ok {
		deny, err := parseSection(denySection, sectionOptions{
			name: "deny",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse deny: %w", err)
		}
		cfg.DenyPatterns = append(cfg.DenyPatterns, deny...)
	}

	return cfg, nil
}

// loadEmbeddedDefaults returns an empty config that denies all commands.
// This ensures mmi rejects everything when no config file exists.
func loadEmbeddedDefaults() *Config {
	return &Config{}
}

// Init loads configuration from files.
// If loading fails, it falls back to embedded defaults.
// Note: This does not auto-create config files. Use EnsureConfigFiles() if needed.
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

	configPath := filepath.Join(configDir, "config.toml")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		logger.Debug("failed to read config file, using embedded defaults", "path", configPath, "error", err)
		globalConfig = loadEmbeddedDefaults()
		configInitialized = true
		return fmt.Errorf("failed to read config.toml: %w", err)
	}

	globalConfig, err = LoadConfigWithDir(configData, configDir)
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
