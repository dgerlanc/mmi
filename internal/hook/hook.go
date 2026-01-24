// Package hook implements the core command approval logic for mmi.
package hook

import (
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/dgerlanc/mmi/internal/audit"
	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/logger"
)

// Result contains the outcome of processing a command.
type Result struct {
	Command  string // The command that was processed
	Approved bool   // Whether the command was approved
	Reason   string // The reason for approval/denial
	Output   string // The JSON output sent to Claude Code
}

// ToolInputData represents the tool_input field in the Claude Code hook input
type ToolInputData struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"` // optional
	Timeout     int    `json:"timeout,omitempty"`     // optional
}

// Input represents the JSON input from Claude Code
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

// Output represents the approval JSON output
type Output struct {
	HookSpecificOutput SpecificOutput `json:"hookSpecificOutput"`
}

// SpecificOutput contains the permission decision
type SpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Read a command and return whether it should be approved and the reason.
// Returns false for parse errors, non-Bash tools, dangerous patterns, or unsafe commands.
func Process(r io.Reader) (approved bool, reason string) {
	result := ProcessWithResult(r)
	return result.Approved, result.Reason
}

// ProcessWithResult reads from a stream and returns a Result with full details.
// This is useful when the caller needs the original command for logging.
func ProcessWithResult(r io.Reader) Result {
	startTime := time.Now()

	// Read raw JSON first so we can log it
	rawBytes, err := io.ReadAll(r)
	if err != nil {
		logger.Debug("failed to read input", "error", err)
		output := FormatAsk("failed to read input")
		return Result{Output: output}
	}
	rawInput := string(rawBytes)

	var input Input
	if err := json.Unmarshal(rawBytes, &input); err != nil {
		logger.Debug("failed to decode input", "error", err)
		output := FormatAsk("invalid input")
		return Result{Output: output}
	}

	if input.ToolName != "Bash" {
		logger.Debug("not a Bash command", "tool", input.ToolName)
		output := FormatAsk("not a Bash command")
		return Result{Output: output}
	}

	cmd := input.ToolInput.Command
	logger.Debug("processing command", "command", cmd)

	cfg := config.Get()

	cmdSegments, err := SplitCommandChain(cmd)
	if err != nil {
		logger.Debug("rejected unparseable command", "command", cmd)
		durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
		segments := []audit.Segment{{
			Command:   cmd,
			Approved:  false,
			Rejection: &audit.Rejection{Code: audit.CodeUnparseable, Detail: "parse error"},
		}}
		output := FormatAsk("unparseable command")
		logAudit(cmd, false, segments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
		return Result{Command: cmd, Approved: false, Reason: "unparseable command", Output: output}
	}
	logger.Debug("split command chain", "segments", len(cmdSegments))

	var reasons []string
	var auditSegments []audit.Segment
	overallApproved := true

	// Evaluate ALL segments - don't return early on rejection
	for i, segment := range cmdSegments {
		coreCmd, wrappers := StripWrappers(segment, cfg.WrapperPatterns)
		logger.Debug("processing segment",
			"index", i,
			"segment", segment,
			"core", coreCmd,
			"wrappers", wrappers)

		// Check for dangerous patterns (command substitution) in this segment
		if containsDangerousPattern(segment) {
			logger.Debug("rejected dangerous pattern in segment", "segment", segment)
			overallApproved = false
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: false,
				Wrappers: wrappers,
				Rejection: &audit.Rejection{
					Code:    audit.CodeCommandSubstitution,
					Pattern: "$(...)",
				},
			})
			continue
		}

		// Check deny list on core command (after splitting chain and stripping wrappers)
		denyResult := CheckDeny(coreCmd, cfg.DenyPatterns)
		if denyResult.Denied {
			logger.Debug("rejected by deny list", "command", coreCmd, "reason", denyResult.Name)
			overallApproved = false
			auditSegments = append(auditSegments, audit.Segment{
				Command:  segment,
				Approved: false,
				Wrappers: wrappers,
				Rejection: &audit.Rejection{
					Code:    audit.CodeDenyMatch,
					Name:    denyResult.Name,
					Pattern: denyResult.Pattern,
				},
			})
			continue
		}

		// Check safe patterns
		safeResult := CheckSafe(coreCmd, cfg.SafeCommands)
		if !safeResult.Matched {
			logger.Debug("rejected unsafe command", "command", coreCmd)
			overallApproved = false
			auditSegments = append(auditSegments, audit.Segment{
				Command:   segment,
				Approved:  false,
				Wrappers:  wrappers,
				Rejection: &audit.Rejection{Code: audit.CodeNoMatch},
			})
			continue
		}

		logger.Debug("matched pattern", "command", coreCmd, "pattern", safeResult.Name)

		// Approved segment
		auditSegments = append(auditSegments, audit.Segment{
			Command:  segment,
			Approved: true,
			Wrappers: wrappers,
			Match: &audit.Match{
				Type:    safeResult.Type,
				Name:    safeResult.Name,
				Pattern: safeResult.Pattern,
			},
		})

		if len(wrappers) > 0 {
			reasons = append(reasons, strings.Join(wrappers, "+")+" + "+safeResult.Name)
		} else {
			reasons = append(reasons, safeResult.Name)
		}
	}

	// Log and return based on overall result
	durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0
	if !overallApproved {
		output := FormatAsk("command not in allow list")
		logAudit(cmd, false, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
		return Result{Command: cmd, Approved: false, Output: output}
	}
	reason := strings.Join(reasons, " | ")
	logger.Debug("approved", "reason", reason)
	output := FormatApproval(reason)
	logAudit(cmd, true, auditSegments, durationMs, input.SessionID, input.ToolUseID, input.Cwd, rawInput, output)
	return Result{Command: cmd, Approved: true, Reason: reason, Output: output}
}

// logAudit logs a command decision to the audit log.
func logAudit(command string, approved bool, segments []audit.Segment, durationMs float64, sessionID, toolUseID, cwd, rawInput, rawOutput string) {
	audit.Log(audit.Entry{
		Version:    1,
		SessionID:  sessionID,
		ToolUseID:  toolUseID,
		Command:    command,
		Approved:   approved,
		Segments:   segments,
		DurationMs: durationMs,
		Cwd:        cwd,
		Input:      rawInput,
		Output:     rawOutput,
	})
}
