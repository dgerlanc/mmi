package main

import (
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/hook"
)

// getTestConfig returns a test configuration with default patterns
func getTestConfig() *config.Config {
	return config.Get()
}

// FuzzSplitCommandChain tests the command chain splitting for crashes
func FuzzSplitCommandChain(f *testing.F) {
	// Add seed corpus
	f.Add("git status")
	f.Add("git status && echo done")
	f.Add("echo 'hello && world'")
	f.Add("ls | grep foo | wc -l")
	f.Add("echo \"test\" && ls -la")
	f.Add("VAR=value cmd")
	f.Add("timeout 30 pytest")
	f.Add("")
	f.Add("   ")
	f.Add("$(cat /etc/passwd)")
	f.Add("`whoami`")
	f.Add("echo ${PATH}")
	f.Add("for i in 1 2 3; do echo $i; done")
	f.Add("if [ -f foo ]; then cat foo; fi")

	f.Fuzz(func(t *testing.T, cmd string) {
		// Just ensure no panics
		_ = hook.SplitCommandChain(cmd)
	})
}

// FuzzProcess tests the full command processing for crashes
func FuzzProcess(f *testing.F) {
	// Add seed corpus with valid JSON inputs
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"git status"}}`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"ls -la"}}`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"echo hello"}}`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"$(whoami)"}}`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":""}}`)
	f.Add(`{"tool_name":"Read","tool_input":{}}`)
	f.Add(`{}`)
	f.Add(`not json`)
	f.Add(`{"tool_name":"Bash","tool_input":{"command":"sudo rm -rf /"}}`)

	f.Fuzz(func(t *testing.T, input string) {
		// Just ensure no panics
		_ = hook.ProcessWithResult(strings.NewReader(input))
	})
}

// FuzzStripWrappers tests wrapper stripping for crashes
func FuzzStripWrappers(f *testing.F) {
	// Add seed corpus
	f.Add("timeout 30 pytest")
	f.Add("env VAR=value cmd")
	f.Add("nice -n 10 cmd")
	f.Add(".venv/bin/python script.py")
	f.Add("/path/to/.venv/bin/pytest")
	f.Add("ENV_VAR=value OTHER=foo cmd arg")
	f.Add("")
	f.Add("   ")

	f.Fuzz(func(t *testing.T, cmd string) {
		// Get config and test with default patterns
		cfg := getTestConfig()
		_, _ = hook.StripWrappers(cmd, cfg.WrapperPatterns)
	})
}

// FuzzCheckSafe tests safe command checking for crashes
func FuzzCheckSafe(f *testing.F) {
	// Add seed corpus
	f.Add("git status")
	f.Add("git log --oneline")
	f.Add("npm install")
	f.Add("cargo build")
	f.Add("pytest -v")
	f.Add("rm -rf /")
	f.Add("")
	f.Add("unknown-command")

	f.Fuzz(func(t *testing.T, cmd string) {
		cfg := getTestConfig()
		_ = hook.CheckSafe(cmd, cfg.SafeCommands)
	})
}

// FuzzCheckDeny tests deny checking for crashes
func FuzzCheckDeny(f *testing.F) {
	// Add seed corpus
	f.Add("sudo rm -rf /")
	f.Add("su - root")
	f.Add("chmod 777 /")
	f.Add("dd if=/dev/zero of=/dev/sda")
	f.Add("mkfs.ext4 /dev/sda1")
	f.Add("git status")
	f.Add("")

	f.Fuzz(func(t *testing.T, cmd string) {
		cfg := getTestConfig()
		_ = hook.CheckDeny(cmd, cfg.DenyPatterns)
	})
}
