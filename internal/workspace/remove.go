package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

func RemoveSkill(target string, expected *LocalSkill, commit func() error) error {
	target = filepath.Clean(target)
	transaction, err := os.MkdirTemp(filepath.Dir(target), ".gh-skill-linker-uninstall-")
	if err != nil {
		return fmt.Errorf("create uninstall staging directory: %w", err)
	}
	preserveTransaction := false
	defer func() {
		if !preserveTransaction {
			_ = os.RemoveAll(transaction)
		}
	}()

	backup := filepath.Join(transaction, "skill")
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("move skill to uninstall staging directory: %w", err)
	}
	rollback := func(cause error) error {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			preserveTransaction = true
			return fmt.Errorf("%w; rollback failed: %v; original preserved at %s", cause, rollbackErr, backup)
		}
		return cause
	}

	if expected != nil {
		current, readErr := ReadSkill(backup)
		if readErr != nil || !sameLocalSkill(current, *expected) {
			if readErr != nil {
				return rollback(fmt.Errorf("%w: re-read current skill: %v", ErrWorkspaceChanged, readErr))
			}
			return rollback(ErrWorkspaceChanged)
		}
	}
	if commit != nil {
		if err := commit(); err != nil {
			return rollback(fmt.Errorf("commit uninstalled skill: %w", err))
		}
	}
	if err := os.RemoveAll(transaction); err != nil {
		return fmt.Errorf("remove uninstall staging directory: %w", err)
	}
	return nil
}
