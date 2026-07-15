package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
)

var lfsPointerSignature = []byte("version https://git-lfs.github.com/spec/v1\n")

func InstallSkill(target string, snapshot source.SkillSnapshot) error {
	if err := ValidateSnapshot(snapshot); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("install destination already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect install destination: %w", err)
	}

	parent := filepath.Dir(filepath.Clean(target))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create skill parent directory: %w", err)
	}
	transaction, err := os.MkdirTemp(parent, ".gh-linked-skills-install-")
	if err != nil {
		return fmt.Errorf("create install staging directory: %w", err)
	}
	defer os.RemoveAll(transaction)
	staged := filepath.Join(transaction, "skill")
	if err := os.Mkdir(staged, 0o755); err != nil {
		return fmt.Errorf("create staged skill: %w", err)
	}
	for relative, content := range snapshot.Files {
		destination := filepath.Join(staged, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create staged directory for %s: %w", relative, err)
		}
		mode := os.FileMode(0o644)
		if snapshot.Executable[relative] {
			mode = 0o755
		}
		if err := os.WriteFile(destination, content, mode); err != nil {
			return fmt.Errorf("write staged file %s: %w", relative, err)
		}
	}
	if err := os.Rename(staged, target); err != nil {
		return fmt.Errorf("activate installed skill: %w", err)
	}
	return nil
}

func ValidateSnapshot(snapshot source.SkillSnapshot) error {
	if _, ok := snapshot.Files["SKILL.md"]; !ok {
		return fmt.Errorf("remote skill has no SKILL.md")
	}
	for relative, content := range snapshot.Files {
		if err := validateRelativeFile(relative); err != nil {
			return err
		}
		if bytes.HasPrefix(content, lfsPointerSignature) {
			return fmt.Errorf("unsupported Git LFS pointer: %s", relative)
		}
	}
	return nil
}

func ExactSnapshot(local LocalSkill, remote source.SkillSnapshot) bool {
	if len(local.Files) != len(remote.Files) || len(local.Executable) != len(remote.Executable) {
		return false
	}
	for relative, content := range local.Files {
		remoteContent, ok := remote.Files[relative]
		if !ok || !bytes.Equal(content, remoteContent) || local.Executable[relative] != remote.Executable[relative] {
			return false
		}
	}
	return true
}
