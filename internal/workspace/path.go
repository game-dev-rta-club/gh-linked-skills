package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafePath = errors.New("unsafe workspace path")

func EnsureContained(projectRoot, target string, allowMissing bool) error {
	root, err := filepath.Abs(filepath.Clean(projectRoot))
	if err != nil {
		return fmt.Errorf("%w: resolve project root: %v", ErrUnsafePath, err)
	}
	target, err = filepath.Abs(filepath.Clean(target))
	if err != nil {
		return fmt.Errorf("%w: resolve target: %v", ErrUnsafePath, err)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%w: target escapes project root", ErrUnsafePath)
	}
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) && allowMissing {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: inspect project root: %v", ErrUnsafePath, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("%w: project root is not a regular directory", ErrUnsafePath)
	}
	current := root
	components := strings.Split(relative, string(filepath.Separator))
	for index, component := range components {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, fs.ErrNotExist) && allowMissing {
			return nil
		}
		if statErr != nil {
			return fmt.Errorf("%w: inspect %s: %v", ErrUnsafePath, current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink component %s", ErrUnsafePath, current)
		}
		if index < len(components)-1 && !info.IsDir() {
			return fmt.Errorf("%w: non-directory component %s", ErrUnsafePath, current)
		}
	}
	return nil
}
