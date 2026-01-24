package hook

/*
Type Relationships in the hook package:

Data Flow:
  Input (JSON from Claude Code)
    → ProcessWithResult()
      → SplitCommandChain() → command segments
      → StripWrappers() → core command + wrapper names
      → CheckDeny() → DenyResult
      → CheckSafe() → SafeResult
    → Result (returned to caller)
    → Output (JSON to Claude Code)

Related packages:
  - config.Config: Provides patterns for matching (WrapperPatterns, SafeCommands, DenyPatterns)
  - patterns.Pattern: Individual compiled regex patterns used for matching
  - audit.Entry: Logged for each command decision with segment-level details
*/

// Input represents the JSON input received from Claude Code's PreToolUse hook.
// This is the entry point for command approval decisions.
//
// See: https://docs.anthropic.com/en/docs/claude-code/hooks
type Input struct {
	SessionID      string        `json:"session_id"`
	TranscriptPath string        `json:"transcript_path"`
	Cwd            string        `json:"cwd"`
	PermissionMode string        `json:"permission_mode"`
	HookEventName  string        `json:"hook_event_name"`
	ToolName       string        `json:"tool_name"`
	ToolInput      ToolInputData `json:"tool_input"`
	ToolUseID      string        `json:"tool_use_id"`
}

// ToolInputData contains the command details from the Bash tool.
// Nested within Input.ToolInput.
type ToolInputData struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Timeout     int    `json:"timeout,omitempty"`
}

// Output represents the JSON response sent back to Claude Code.
// Wraps SpecificOutput in the expected format.
type Output struct {
	HookSpecificOutput SpecificOutput `json:"hookSpecificOutput"`
}

// SpecificOutput contains the permission decision details.
// PermissionDecision is either "allow" or "ask".
type SpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Result contains the outcome of processing a command.
// Returned by ProcessWithResult() for use by the caller.
type Result struct {
	Command  string // The original command that was processed
	Approved bool   // Whether the command was approved
	Reason   string // Human-readable reason for the decision
	Output   string // JSON output string sent to Claude Code
}

// SafeResult contains detailed information about a safe pattern match.
// Returned by CheckSafe() when evaluating commands against allowed patterns.
type SafeResult struct {
	Matched bool   // Whether a pattern matched
	Name    string // Name of the matched pattern (from config)
	Type    string // Pattern type: "simple", "subcommand", "regex", or "command"
	Pattern string // The regex pattern that matched
}

// DenyResult contains detailed information about a deny pattern match.
// Returned by CheckDeny() when evaluating commands against blocked patterns.
type DenyResult struct {
	Denied  bool   // Whether the command matched a deny pattern
	Name    string // Name of the deny rule (from config)
	Pattern string // The regex pattern that matched
}
