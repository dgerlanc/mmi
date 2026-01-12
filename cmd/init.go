package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/spf13/cobra"
)

var (
	initForce bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new mmi configuration file",
	Long: `Initialize creates a new mmi configuration file with interactive prompts.

This command helps you create a customized config.toml by asking which
categories of commands you want to allow.

The config file is written to ~/.config/mmi/config.toml (or the path
specified by MMI_CONFIG environment variable).`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config without prompting")
}

func runInit(cmd *cobra.Command, args []string) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.toml")

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !initForce {
		if !confirm("Config file already exists. Overwrite?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Interactive prompts
	fmt.Println("MMI Configuration Generator")
	fmt.Println("============================")
	fmt.Println()

	includeGit := confirm("Include git commands (diff, log, status, etc.)?")
	includePython := confirm("Include Python tools (pytest, python, uv, pip)?")
	includeNode := confirm("Include Node.js tools (npm, npx)?")
	includeRust := confirm("Include Rust tools (cargo)?")
	includeGo := confirm("Include Go tools (go)?")
	includeReadOnly := confirm("Include read-only utilities (ls, cat, grep, etc.)?")
	includeDeny := confirm("Include deny list for dangerous commands (sudo, rm -rf /, etc.)?")

	// Generate config
	var b strings.Builder

	b.WriteString(`# mmi configuration - safe commands and wrappers for Claude Code bash approval
#
# Structure:
#   [deny.*]     - patterns that are ALWAYS rejected (checked first)
#   [wrappers.*] - prefixes stripped before checking the core command
#   [commands.*] - safe commands that are allowed to execute
#
# Section types:
#   [[*.simple]]     - name = "label", commands = [...] - any arguments allowed
#   [[*.command]]    - command = "cmd", flags = [...] - wrapper with flags
#   [[*.subcommand]] - command = "cmd", subcommands = [...], flags = [...]
#   [[*.regex]]      - pattern = "^regex$", name = "desc" - raw regex escape hatch

`)

	// Deny section
	if includeDeny {
		b.WriteString(`# ============================================================
# DENY LIST - patterns that are always rejected (checked first)
# These override any approval patterns below
# ============================================================

[[deny.simple]]
name = "privilege escalation"
commands = ["sudo", "su", "doas"]

[[deny.regex]]
pattern = 'rm\s+(-[rRfF]+\s+)*/'
name = "rm root"

[[deny.regex]]
pattern = 'chmod\s+(777|a\+rwx)'
name = "chmod world-writable"

[[deny.regex]]
pattern = 'dd\s+.*of=/dev/'
name = "dd to device"

[[deny.regex]]
pattern = '>\s*/dev/sd[a-z]'
name = "write to disk"

[[deny.regex]]
pattern = 'mkfs\.'
name = "format filesystem"

[[deny.regex]]
pattern = ':(){:|:&};:'
name = "fork bomb"

`)
	}

	// Wrappers section
	b.WriteString(`# ============================================================
# WRAPPERS - prefixes stripped before checking the core command
# ============================================================

[[wrappers.simple]]
name = "env"
commands = ["env", "do"]

[[wrappers.command]]
command = "timeout"
flags = ["<arg>"]

[[wrappers.command]]
command = "nice"
flags = ["-n <arg>", ""]

[[wrappers.regex]]
pattern = '^([A-Z_][A-Z0-9_]*=[^\s]*\s+)+'
name = "env vars"

[[wrappers.regex]]
pattern = '^(\.\./)*\.?venv/bin/'
name = ".venv"

[[wrappers.regex]]
pattern = '^/[^\s]+/\.?venv/bin/'
name = ".venv"

`)

	// Commands section
	b.WriteString(`# ============================================================
# COMMANDS - safe commands that are allowed to execute
# ============================================================

`)

	// Git
	if includeGit {
		b.WriteString(`[[commands.subcommand]]
command = "git"
subcommands = ["diff", "log", "status", "show", "branch", "stash", "bisect", "fetch", "add", "checkout", "merge", "rebase", "worktree", "commit"]
flags = ["-C <arg>"]

`)
	}

	// Python
	if includePython {
		b.WriteString(`[[commands.simple]]
name = "python"
commands = ["pytest", "python", "ruff", "uvx"]

[[commands.subcommand]]
command = "uv"
subcommands = ["pip", "run", "sync", "venv", "add", "remove", "lock"]

`)
	}

	// Node
	if includeNode {
		b.WriteString(`[[commands.simple]]
name = "node"
commands = ["npx"]

[[commands.subcommand]]
command = "npm"
subcommands = ["install", "run", "test", "build", "ci"]

`)
	}

	// Rust
	if includeRust {
		b.WriteString(`[[commands.subcommand]]
command = "cargo"
subcommands = ["build", "test", "run", "check", "clippy", "fmt", "clean"]

[[commands.subcommand]]
command = "maturin"
subcommands = ["develop", "build"]

`)
	}

	// Go
	if includeGo {
		b.WriteString(`[[commands.simple]]
name = "go"
commands = ["go"]

`)
	}

	// Read-only utilities
	if includeReadOnly {
		b.WriteString(`[[commands.simple]]
name = "read-only"
commands = ["ls", "cat", "head", "tail", "wc", "find", "grep", "rg", "file", "which", "pwd", "du", "df", "curl", "sort", "uniq", "cut", "tr", "awk", "sed", "xargs"]

`)
	}

	// Common simple commands
	b.WriteString(`[[commands.simple]]
name = "common"
commands = ["make", "touch", "echo", "sleep"]

[[commands.simple]]
name = "process-mgmt"
commands = ["pkill", "kill"]

[[commands.simple]]
name = "loops"
commands = ["done"]

# Shell builtins for control flow
[[commands.regex]]
pattern = '^(true|false|exit(\s+\d+)?)$'
name = "shell builtin"

[[commands.regex]]
pattern = '^cd\s'
name = "cd"

[[commands.regex]]
pattern = '^(source|\.) [^\s]*venv/bin/activate'
name = "venv activate"

[[commands.regex]]
pattern = '^[A-Z_][A-Z0-9_]*=\S*$'
name = "var assignment"

[[commands.regex]]
pattern = '^for\s+\w+\s+in\s'
name = "for loop"

[[commands.regex]]
pattern = '^while\s'
name = "while loop"
`)

	// Create directory if needed
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Println()
	fmt.Printf("Configuration written to: %s\n", configPath)
	fmt.Println()
	fmt.Println("Run 'mmi validate' to verify your configuration.")

	return nil
}

// confirm prompts the user with a yes/no question (default yes)
func confirm(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [Y/n] ", prompt)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "" || response == "y" || response == "yes"
}
