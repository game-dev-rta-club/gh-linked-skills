package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/syncstate"
)

var (
	ErrInvalidSkill    = errors.New("invalid skill directory")
	ErrUnsupportedFile = errors.New("unsupported file")
)

type LocalSkill struct {
	Files      map[string][]byte
	Executable map[string]bool
	Snapshot   syncstate.Snapshot
}

func ReadSkill(root string) (LocalSkill, error) {
	root = filepath.Clean(root)
	documentPath := filepath.Join(root, "SKILL.md")
	if info, err := os.Lstat(documentPath); err != nil || !info.Mode().IsRegular() {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return LocalSkill{}, fmt.Errorf("inspect SKILL.md: %w", err)
		}
		return LocalSkill{}, fmt.Errorf("%w: regular SKILL.md is required", ErrInvalidSkill)
	}

	result := LocalSkill{
		Files:      make(map[string][]byte),
		Executable: make(map[string]bool),
		Snapshot:   make(syncstate.Snapshot),
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s", ErrUnsupportedFile, path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		result.Files[relative] = append([]byte(nil), content...)
		result.Executable[relative] = info.Mode().Perm()&0o111 != 0
		result.Snapshot[relative] = content
		return nil
	})
	if err != nil {
		return LocalSkill{}, fmt.Errorf("read skill %s: %w", root, err)
	}
	return result, nil
}
