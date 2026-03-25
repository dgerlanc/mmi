package cmdpath

func init() {
	register(CommandDescriptor{
		Name:           "rm",
		ExtractTargets: extractRmTargets,
	})
	register(CommandDescriptor{
		Name:           "mv",
		ExtractTargets: extractMvTargets,
	})
	register(CommandDescriptor{
		Name:           "chmod",
		ExtractTargets: extractChmodTargets,
	})
	register(CommandDescriptor{
		Name:           "chown",
		ExtractTargets: extractChownTargets,
	})
}

// Stub implementations — filled in later tasks
func extractRmTargets(args []string) ([]string, []string)    { return nil, nil }
func extractMvTargets(args []string) ([]string, []string)    { return nil, nil }
func extractChmodTargets(args []string) ([]string, []string) { return nil, nil }
func extractChownTargets(args []string) ([]string, []string) { return nil, nil }
