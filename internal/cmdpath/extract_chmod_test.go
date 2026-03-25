package cmdpath

import (
	"reflect"
	"testing"
)

func TestExtractChmodTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "numeric mode",
			args:        []string{"755", "file.sh"},
			wantTargets: []string{"file.sh"},
		},
		{
			name:        "symbolic mode",
			args:        []string{"u+x", "file.sh"},
			wantTargets: []string{"file.sh"},
		},
		{
			name:        "4-digit numeric mode",
			args:        []string{"0644", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "complex symbolic mode",
			args:        []string{"go-rwx", "secret.key"},
			wantTargets: []string{"secret.key"},
		},
		{
			name:        "multiple files",
			args:        []string{"644", "a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags before mode",
			args:        []string{"-R", "755", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"755", "--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:           "variable in path",
			args:           []string{"755", "$DIR/file"},
			wantUnresolved: []string{"$DIR/file"},
		},
		{
			name: "no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractChmodTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}

func TestExtractChownTargets(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantTargets    []string
		wantUnresolved []string
	}{
		{
			name:        "user only",
			args:        []string{"root", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "user:group",
			args:        []string{"root:wheel", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "user with dot",
			args:        []string{"user.name", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name:        "multiple files",
			args:        []string{"nobody", "a.txt", "b.txt"},
			wantTargets: []string{"a.txt", "b.txt"},
		},
		{
			name:        "flags before owner",
			args:        []string{"-R", "root", "dir/"},
			wantTargets: []string{"dir/"},
		},
		{
			name:        "double dash",
			args:        []string{"root", "--", "-weird-file"},
			wantTargets: []string{"-weird-file"},
		},
		{
			name:           "variable in path",
			args:           []string{"root", "$DIR/file"},
			wantUnresolved: []string{"$DIR/file"},
		},
		{
			name:        "colon-only group",
			args:        []string{":wheel", "file.txt"},
			wantTargets: []string{"file.txt"},
		},
		{
			name: "no args",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, unresolved := extractChownTargets(tt.args)
			if !reflect.DeepEqual(targets, tt.wantTargets) {
				t.Errorf("targets = %v, want %v", targets, tt.wantTargets)
			}
			if !reflect.DeepEqual(unresolved, tt.wantUnresolved) {
				t.Errorf("unresolved = %v, want %v", unresolved, tt.wantUnresolved)
			}
		})
	}
}
