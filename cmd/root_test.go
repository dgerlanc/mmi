package cmd

import (
	"testing"
)

func TestRootCmdHasExpectedSubcommands(t *testing.T) {
	rootCmd := buildRootCmd()
	subcommands := make(map[string]bool)
	for _, cmd := range rootCmd.Commands() {
		subcommands[cmd.Name()] = true
	}

	expected := []string{"validate", "init", "completion"}
	for _, name := range expected {
		if !subcommands[name] {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestRootCmdUsageContainsDescription(t *testing.T) {
	rootCmd := buildRootCmd()
	if rootCmd.Short == "" {
		t.Error("root command should have a short description")
	}
	if rootCmd.Long == "" {
		t.Error("root command should have a long description")
	}
}
