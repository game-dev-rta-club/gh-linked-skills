package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureContainedRejectsEscapeAndSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
		t.Fatal(err)
	}

	tests := []string{
		filepath.Join(root, "..", "outside"),
		filepath.Join(root, ".agents", "skills", "sample"),
	}
	for _, target := range tests {
		if err := EnsureContained(root, target, true); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("EnsureContained(%q) error = %v, want ErrUnsafePath", target, err)
		}
	}
}

func TestEnsureContainedAllowsMissingRegularPath(t *testing.T) {
	root := t.TempDir()
	if err := EnsureContained(root, filepath.Join(root, ".agents", "skills", "sample"), true); err != nil {
		t.Fatalf("EnsureContained() error = %v", err)
	}
}
