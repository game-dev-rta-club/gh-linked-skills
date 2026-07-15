package merge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

var ErrUnsupportedConflict = errors.New("unsupported conflict")

type FileMerger interface {
	MergeFile(
		ctx context.Context,
		local []byte,
		base []byte,
		remote []byte,
		baseSHA string,
		remoteSHA string,
	) (merged []byte, conflict bool, err error)
}

func ThreeWay(
	ctx context.Context,
	merger FileMerger,
	base source.SkillSnapshot,
	local workspace.LocalSkill,
	remote source.SkillSnapshot,
) (source.SkillSnapshot, []string, error) {
	paths := unionPaths(base.Files, local.Files, remote.Files)
	result := source.SkillSnapshot{
		TreeSHA:    remote.TreeSHA,
		Files:      make(map[string][]byte),
		Executable: make(map[string]bool),
	}
	conflictPaths := make([]string, 0)
	for _, filePath := range paths {
		baseContent, baseExists := base.Files[filePath]
		localContent, localExists := local.Files[filePath]
		remoteContent, remoteExists := remote.Files[filePath]
		merged, executable, conflict, keep, err := mergePath(
			ctx, merger, filePath,
			baseContent, baseExists, base.Executable[filePath],
			localContent, localExists, local.Executable[filePath],
			remoteContent, remoteExists, remote.Executable[filePath],
			base.TreeSHA, remote.TreeSHA,
		)
		if err != nil {
			return source.SkillSnapshot{}, nil, err
		}
		if !keep {
			continue
		}
		result.Files[filePath] = merged
		result.Executable[filePath] = executable
		if conflict {
			conflictPaths = append(conflictPaths, filePath)
		}
	}
	if _, ok := result.Files["SKILL.md"]; !ok {
		return source.SkillSnapshot{}, nil, fmt.Errorf("%w: SKILL.md was deleted", ErrUnsupportedConflict)
	}
	if collision := fileDirectoryCollision(result.Files); collision != "" {
		return source.SkillSnapshot{}, nil, fmt.Errorf("%w: file/directory collision at %s", ErrUnsupportedConflict, collision)
	}
	return result, conflictPaths, nil
}

func mergePath(
	ctx context.Context,
	merger FileMerger,
	filePath string,
	base []byte,
	baseExists bool,
	baseExecutable bool,
	local []byte,
	localExists bool,
	localExecutable bool,
	remote []byte,
	remoteExists bool,
	remoteExecutable bool,
	baseSHA string,
	remoteSHA string,
) ([]byte, bool, bool, bool, error) {
	if baseExists {
		switch {
		case !localExists && !remoteExists:
			return nil, false, false, false, nil
		case !localExists:
			if bytes.Equal(remote, base) && remoteExecutable == baseExecutable {
				return nil, false, false, false, nil
			}
			return nil, false, false, false, fmt.Errorf("%w: modify/delete at %s", ErrUnsupportedConflict, filePath)
		case !remoteExists:
			if bytes.Equal(local, base) && localExecutable == baseExecutable {
				return nil, false, false, false, nil
			}
			return nil, false, false, false, fmt.Errorf("%w: modify/delete at %s", ErrUnsupportedConflict, filePath)
		}
	} else {
		switch {
		case !localExists && !remoteExists:
			return nil, false, false, false, nil
		case localExists && !remoteExists:
			return local, localExecutable, false, true, nil
		case !localExists && remoteExists:
			return remote, remoteExecutable, false, true, nil
		}
	}

	if bytes.Equal(local, remote) {
		return local, mergeExecutableMode(baseExecutable, localExecutable, remoteExecutable), false, true, nil
	}
	if baseExists && bytes.Equal(local, base) {
		return remote, mergeExecutableMode(baseExecutable, localExecutable, remoteExecutable), false, true, nil
	}
	if baseExists && bytes.Equal(remote, base) {
		return local, mergeExecutableMode(baseExecutable, localExecutable, remoteExecutable), false, true, nil
	}
	if isBinary(base) || isBinary(local) || isBinary(remote) {
		return nil, false, false, false, fmt.Errorf("%w: binary conflict at %s", ErrUnsupportedConflict, filePath)
	}
	merged, conflict, err := merger.MergeFile(ctx, local, base, remote, baseSHA, remoteSHA)
	if err != nil {
		return nil, false, false, false, fmt.Errorf("merge %s: %w", filePath, err)
	}
	executable := mergeExecutableMode(baseExecutable, localExecutable, remoteExecutable)
	return merged, executable, conflict, true, nil
}

func unionPaths(maps ...map[string][]byte) []string {
	set := make(map[string]struct{})
	for _, files := range maps {
		for filePath := range files {
			set[filePath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(set))
	for filePath := range set {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths
}

func fileDirectoryCollision(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	for index, filePath := range paths {
		prefix := filePath + "/"
		if index+1 < len(paths) && strings.HasPrefix(paths[index+1], prefix) {
			return filePath
		}
	}
	return ""
}

func isBinary(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0
}

func mergeExecutableMode(base, local, remote bool) bool {
	if local == base {
		return remote
	}
	if remote == base {
		return local
	}
	return local || remote
}
