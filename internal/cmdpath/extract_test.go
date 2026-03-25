package cmdpath

import (
	"reflect"
	"testing"
)

func TestExtractRmTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "simple file",
			args:        []string{"foo.txt"},
			wantTargets: []string{"foo.txt"},
		},
		{
			name:        "multiple files",
			args:        []string{"a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags are skipped",
			args:        []string{"-rf", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "long flags are skipped",
			args:        []string{"--force", "--recursive", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:        "double dash with flags before",
			args:        []string{"-rf", "--", "-file1", "-file2"},
			wantTargets: []string{"-file1", "-file2"},
		},
		{
			name:           "shell variable",
			args:           []string{"$HOME/foo"},
			wantUnresolved: []string{"$HOME/foo"},
		},
		{
			name:           "mixed resolved and unresolved",
			args:           []string{"foo.txt", "$DIR/bar"},
			wantTargets:    []string{"foo.txt"},
			wantUnresolved: []string{"$DIR/bar"},
		},
		{
			name:        "absolute path",
			args:        []string{"-f", "/tmp/foo.txt"},
			wantTargets: []string{"/tmp/foo.txt"},
		},
		{
			name:        "glob pattern",
			args:        []string{"*.log"},
			wantTargets: []string{"*.log"},
		},
		{
			name: "no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractRmTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

func TestExtractMvTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "source and dest",
			args:        []string{"src.txt", "dst.txt"},
			wantTargets: []string{"src.txt", "dst.txt"},
		},
		{
			name:        "flags skipped",
			args:        []string{"-f", "src.txt", "dst.txt"},
			wantTargets: []string{"src.txt", "dst.txt"},
		},
		{
			name:        "double dash",
			args:        []string{"--", "-src", "-dst"},
			wantTargets: []string{"-src", "-dst"},
		},
		{
			name:           "variable in target",
			args:           []string{"src.txt", "$DEST"},
			wantTargets:    []string{"src.txt"},
			wantUnresolved: []string{"$DEST"},
		},
		{
			name:        "long flags",
			args:        []string{"--force", "--backup=numbered", "src", "dst"},
			wantTargets: []string{"src", "dst"},
		},
		{
			name:        "target-directory flag with arg",
			args:        []string{"-t", "/tmp", "src.txt"},
			wantTargets: []string{"/tmp", "src.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractMvTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}
