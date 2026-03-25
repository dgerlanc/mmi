package cmdpath

import (
	"testing"
)

func TestLookupDescriptor(t *testing.T) {
	tests := []struct {
		name    string
		command string
		found   bool
	}{
		{"rm is registered", "rm", true},
		{"mv is registered", "mv", true},
		{"chmod is registered", "chmod", true},
		{"chown is registered", "chown", true},
		{"ls is not registered", "ls", false},
		{"git is not registered", "git", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, ok := LookupDescriptor(tt.command)
			if ok != tt.found {
				t.Errorf("LookupDescriptor(%q) found=%v, want %v", tt.command, ok, tt.found)
			}
			if ok && desc.Name != tt.command {
				t.Errorf("LookupDescriptor(%q) name=%q, want %q", tt.command, desc.Name, tt.command)
			}
		})
	}
}

func TestRegisteredCommands(t *testing.T) {
	cmds := RegisteredCommands()
	if len(cmds) != 4 {
		t.Errorf("expected 4 registered commands, got %d", len(cmds))
	}

	expected := map[string]bool{"rm": true, "mv": true, "chmod": true, "chown": true}
	for _, cmd := range cmds {
		if !expected[cmd] {
			t.Errorf("unexpected registered command: %s", cmd)
		}
	}
}
