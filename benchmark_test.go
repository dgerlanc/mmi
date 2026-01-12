package main

import (
	"strings"
	"testing"

	"github.com/dgerlanc/mmi/internal/config"
	"github.com/dgerlanc/mmi/internal/hook"
)

// BenchmarkSplitCommandChain benchmarks command chain splitting
func BenchmarkSplitCommandChain(b *testing.B) {
	benchmarks := []struct {
		name string
		cmd  string
	}{
		{"simple", "git status"},
		{"chained", "git add . && git commit -m 'test' && git push"},
		{"piped", "cat file.txt | grep foo | wc -l"},
		{"complex", "VAR=value timeout 30 pytest -v tests/ && echo done"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hook.SplitCommandChain(bm.cmd)
			}
		})
	}
}

// BenchmarkProcess benchmarks the full command approval process
func BenchmarkProcess(b *testing.B) {
	// Ensure config is loaded before benchmark
	_ = config.Get()

	benchmarks := []struct {
		name  string
		input string
	}{
		{"simple_approved", `{"tool_name":"Bash","tool_input":{"command":"git status"}}`},
		{"simple_rejected", `{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`},
		{"chained_approved", `{"tool_name":"Bash","tool_input":{"command":"git status && git log"}}`},
		{"with_wrapper", `{"tool_name":"Bash","tool_input":{"command":"timeout 30 pytest -v"}}`},
		{"non_bash", `{"tool_name":"Read","tool_input":{"file_path":"/tmp/test"}}`},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hook.ProcessWithResult(strings.NewReader(bm.input))
			}
		})
	}
}

// BenchmarkStripWrappers benchmarks wrapper stripping
func BenchmarkStripWrappers(b *testing.B) {
	cfg := config.Get()

	benchmarks := []struct {
		name string
		cmd  string
	}{
		{"no_wrapper", "pytest -v"},
		{"single_wrapper", "timeout 30 pytest -v"},
		{"double_wrapper", "env timeout 30 pytest -v"},
		{"env_vars", "VAR=value OTHER=foo pytest -v"},
		{"venv", ".venv/bin/python script.py"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = hook.StripWrappers(bm.cmd, cfg.WrapperPatterns)
			}
		})
	}
}

// BenchmarkCheckSafe benchmarks safe command checking
func BenchmarkCheckSafe(b *testing.B) {
	cfg := config.Get()

	benchmarks := []struct {
		name string
		cmd  string
	}{
		{"git", "git status"},
		{"npm", "npm install"},
		{"simple", "pytest -v"},
		{"regex", "for i in 1 2 3; do echo $i; done"},
		{"no_match", "unknown-command"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hook.CheckSafe(bm.cmd, cfg.SafeCommands)
			}
		})
	}
}

// BenchmarkCheckDeny benchmarks deny pattern checking
func BenchmarkCheckDeny(b *testing.B) {
	cfg := config.Get()

	benchmarks := []struct {
		name string
		cmd  string
	}{
		{"denied_simple", "sudo rm -rf /"},
		{"denied_regex", "chmod 777 /tmp"},
		{"allowed", "git status"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = hook.CheckDeny(bm.cmd, cfg.DenyPatterns)
			}
		})
	}
}
