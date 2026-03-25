package cmdpath

import (
	"testing"
)

func TestExpandPathVariables(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		cwd     string
		gitRoot string
		want    []string
	}{
		{
			name:    "expand $PROJECT",
			paths:   []string{"$PROJECT"},
			cwd:     "/home/user/project",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project"},
		},
		{
			name:    "expand $PROJECT_ROOT",
			paths:   []string{"$PROJECT_ROOT"},
			cwd:     "/home/user/project/.claude/worktrees/feat",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project"},
		},
		{
			name:    "literal path unchanged",
			paths:   []string{"/tmp"},
			cwd:     "/home/user/project",
			gitRoot: "/home/user/project",
			want:    []string{"/tmp"},
		},
		{
			name:    "mixed variables and literals",
			paths:   []string{"$PROJECT", "/tmp", "$PROJECT_ROOT"},
			cwd:     "/home/user/project/.claude/worktrees/feat",
			gitRoot: "/home/user/project",
			want:    []string{"/home/user/project/.claude/worktrees/feat", "/tmp", "/home/user/project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPathVariables(tt.paths, tt.cwd, tt.gitRoot)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d paths, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("path[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantResolved bool
	}{
		{"tilde only", "~", true},
		{"tilde with path", "~/foo/bar", true},
		{"tilde with user", "~bob/foo", false},
		{"absolute path", "/absolute/path", true},
		{"relative path", "relative/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, resolved := ExpandTilde(tt.path)
			if resolved != tt.wantResolved {
				t.Errorf("resolved = %v, want %v", resolved, tt.wantResolved)
			}
			if !resolved {
				return
			}
			// For tilde paths, just verify it starts with a non-tilde character
			if tt.path == "~" || (len(tt.path) > 1 && tt.path[1] == '/') {
				if len(got) == 0 || got[0] == '~' {
					t.Errorf("tilde was not expanded: %q", got)
				}
			}
		})
	}
}

func TestResolveTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		cwd     string
		want    []string
	}{
		{
			name:    "absolute path unchanged",
			targets: []string{"/tmp/foo"},
			cwd:     "/home/user",
			want:    []string{"/tmp/foo"},
		},
		{
			name:    "relative path resolved",
			targets: []string{"foo.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/foo.txt"},
		},
		{
			name:    "dot-dot resolved",
			targets: []string{"../bar.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/bar.txt"},
		},
		{
			name:    "dot resolved",
			targets: []string{"./foo.txt"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/foo.txt"},
		},
		{
			name:    "glob base directory",
			targets: []string{"*.log"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/*.log"},
		},
		{
			name:    "glob with directory",
			targets: []string{"subdir/*.log"},
			cwd:     "/home/user/project",
			want:    []string{"/home/user/project/subdir/*.log"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveTargets(tt.targets, tt.cwd)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d targets, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("target[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCheckPathPrefixes(t *testing.T) {
	tests := []struct {
		name    string
		targets []string
		allowed []string
		ok      bool
	}{
		{
			name:    "target under allowed",
			targets: []string{"/home/user/project/foo.txt"},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "target is exactly allowed",
			targets: []string{"/home/user/project"},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "target outside allowed",
			targets: []string{"/etc/passwd"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "one of multiple allowed",
			targets: []string{"/tmp/foo"},
			allowed: []string{"/home/user/project", "/tmp"},
			ok:      true,
		},
		{
			name:    "mixed: one in, one out",
			targets: []string{"/home/user/project/ok.txt", "/etc/bad"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "no targets is ok",
			targets: []string{},
			allowed: []string{"/home/user/project"},
			ok:      true,
		},
		{
			name:    "prefix must be directory boundary",
			targets: []string{"/home/user/project-other/foo.txt"},
			allowed: []string{"/home/user/project"},
			ok:      false,
		},
		{
			name:    "trailing slash on allowed",
			targets: []string{"/home/user/project/foo.txt"},
			allowed: []string{"/home/user/project/"},
			ok:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckPathPrefixes(tt.targets, tt.allowed)
			if got != tt.ok {
				t.Errorf("CheckPathPrefixes(%v, %v) = %v, want %v", tt.targets, tt.allowed, got, tt.ok)
			}
		})
	}
}
