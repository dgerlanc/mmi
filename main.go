// mmi (mother may I?) - Claude Code PreToolUse Hook for Bash Command Approval
//
// This hook auto-approves Bash commands that are safe combinations of:
//
//	WRAPPERS (timeout, env vars, .venv/bin/) + CORE COMMANDS (git, pytest, etc.)
//
// Usage in ~/.claude/settings.json:
//
//	"hooks": {
//	  "PreToolUse": [{
//	    "matcher": "Bash",
//	    "hooks": [{"type": "command", "command": "mmi"}]
//	  }]
//	}
//
// Test:
//
//	echo '{"tool_name": "Bash", "tool_input": {"command": "timeout 30 pytest"}}' | mmi
package main

import (
	"os"

	"github.com/dgerlanc/mmi/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
