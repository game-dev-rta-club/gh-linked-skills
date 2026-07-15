package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

var ErrWorkspaceChanged = errors.New("workspace changed during pull")

func ReplaceExact(target string, remote source.SkillSnapshot, expected LocalSkill, commit func() error) error {
	document, ok := remote.Files["SKILL.md"]
	if !ok {
		return fmt.Errorf("remote skill has no SKILL.md")
	}
	return replaceSkill(target, remote, document, expected, commit)
}

func replaceSkill(target string, remote source.SkillSnapshot, document []byte, expected LocalSkill, commit func() error) error {
	if remote.TreeSHA == "" {
		return fmt.Errorf("remote skill tree SHA is required")
	}
	if _, ok := remote.Files["SKILL.md"]; !ok {
		return fmt.Errorf("remote skill has no SKILL.md")
	}
	for relative := range remote.Files {
		if err := validateRelativeFile(relative); err != nil {
			return err
		}
	}

	target = filepath.Clean(target)
	parent := filepath.Dir(target)
	transaction, err := os.MkdirTemp(parent, ".gh-skill-linker-pull-")
	if err != nil {
		return fmt.Errorf("create pull staging directory: %w", err)
	}
	preserveTransaction := false
	defer func() {
		if !preserveTransaction {
			_ = os.RemoveAll(transaction)
		}
	}()

	staged := filepath.Join(transaction, "new")
	if err := os.Mkdir(staged, 0o755); err != nil {
		return fmt.Errorf("create staged skill: %w", err)
	}
	for relative, content := range remote.Files {
		if relative == "SKILL.md" {
			content = document
		}
		destination := filepath.Join(staged, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("create staged directory for %s: %w", relative, err)
		}
		mode := os.FileMode(0o644)
		if remote.Executable[relative] {
			mode = 0o755
		}
		if err := os.WriteFile(destination, content, mode); err != nil {
			return fmt.Errorf("write staged %s: %w", relative, err)
		}
	}

	backup := filepath.Join(transaction, "old")
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("move current skill to rollback location: %w", err)
	}
	current, readErr := ReadSkill(backup)
	if readErr != nil || !sameLocalSkill(current, expected) {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			preserveTransaction = true
			return fmt.Errorf("%w; rollback failed: %v; original preserved at %s", ErrWorkspaceChanged, rollbackErr, backup)
		}
		if readErr != nil {
			return fmt.Errorf("%w: re-read current skill: %v", ErrWorkspaceChanged, readErr)
		}
		return ErrWorkspaceChanged
	}
	if err := os.Rename(staged, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			preserveTransaction = true
			return fmt.Errorf("activate pulled skill: %w; rollback failed: %v; original preserved at %s", err, rollbackErr, backup)
		}
		return fmt.Errorf("activate pulled skill: %w", err)
	}
	if commit != nil {
		if err := commit(); err != nil {
			if removeErr := os.RemoveAll(target); removeErr != nil {
				preserveTransaction = true
				return fmt.Errorf("commit pulled skill: %w; remove replacement during rollback: %v; original preserved at %s", err, removeErr, backup)
			}
			if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
				preserveTransaction = true
				return fmt.Errorf("commit pulled skill: %w; rollback failed: %v; original preserved at %s", err, rollbackErr, backup)
			}
			return fmt.Errorf("commit pulled skill: %w", err)
		}
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove pull rollback directory: %w", err)
	}
	return nil
}

func sameLocalSkill(left, right LocalSkill) bool {
	if !reflect.DeepEqual(left.Executable, right.Executable) ||
		!reflect.DeepEqual(left.EmptyDirectories, right.EmptyDirectories) ||
		len(left.Files) != len(right.Files) {
		return false
	}
	for filePath, content := range left.Files {
		rightContent, ok := right.Files[filePath]
		if !ok || !bytes.Equal(content, rightContent) {
			return false
		}
	}
	return true
}

func validateRelativeFile(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("unsafe remote file path %q", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("unsafe remote file path %q", value)
	}
	return nil
}
