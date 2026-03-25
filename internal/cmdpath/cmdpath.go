// Package cmdpath provides command-specific argument parsing for extracting
// filesystem target paths from shell commands, and path resolution/validation
// against allowed directory prefixes.
package cmdpath

import "sort"

// CommandDescriptor defines how to extract filesystem target paths from a command's arguments.
type CommandDescriptor struct {
	Name           string
	ExtractTargets func(args []string) (targets []string, unresolved []string)
}

// registry maps command names to their descriptors.
var registry = map[string]CommandDescriptor{}

// register adds a descriptor to the registry. Called from extract.go init.
func register(desc CommandDescriptor) {
	registry[desc.Name] = desc
}

// LookupDescriptor returns the descriptor for a command, if registered.
func LookupDescriptor(command string) (CommandDescriptor, bool) {
	desc, ok := registry[command]
	return desc, ok
}

// RegisteredCommands returns a sorted list of all registered command names.
func RegisteredCommands() []string {
	cmds := make([]string, 0, len(registry))
	for name := range registry {
		cmds = append(cmds, name)
	}
	sort.Strings(cmds)
	return cmds
}
