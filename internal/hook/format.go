package hook

import (
	"encoding/json"

	"github.com/dgerlanc/mmi/internal/logger"
)

// FormatApproval returns the JSON approval output
func FormatApproval(reason string) string {
	output := Output{
		HookSpecificOutput: SpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "allow",
			PermissionDecisionReason: reason,
		},
	}
	data, err := json.Marshal(output)
	if err != nil {
		logger.Debug("failed to marshal approval output", "error", err)
		return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"internal error"}}`
	}
	return string(data)
}

// FormatAsk returns the JSON ask output
func FormatAsk(reason string) string {
	output := Output{
		HookSpecificOutput: SpecificOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "ask",
			PermissionDecisionReason: reason,
		},
	}
	data, err := json.Marshal(output)
	if err != nil {
		logger.Debug("failed to marshal ask output", "error", err)
		return `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"internal error"}}`
	}
	return string(data)
}
