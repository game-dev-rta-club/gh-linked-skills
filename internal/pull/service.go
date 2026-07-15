package pull

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-linked-skills/internal/manifest"
	mergeapp "github.com/game-dev-rta-club/gh-linked-skills/internal/merge"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/skill"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/workspace"
)

type Registry interface {
	ListProject(ctx context.Context, projectRoot string) ([]manifest.InstalledSkill, error)
	Advance(projectRoot, name string, expected manifest.Skill, commitSHA, treeSHA string) error
}

type LocalReader interface {
	Read(path string) (workspace.LocalSkill, error)
}

type Remote interface {
	ReadSkill(ctx context.Context, repository source.Repository, skillPath string, revision string) (source.SkillSnapshot, error)
}

type Git interface {
	TrackedFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error)
	MergeFile(ctx context.Context, local, base, remote []byte, baseSHA, remoteSHA string) ([]byte, bool, error)
}

type Writer interface {
	ReplaceExact(path string, remote source.SkillSnapshot, expected workspace.LocalSkill, commit func() error) error
}

type Service struct {
	registry Registry
	local    LocalReader
	remote   Remote
	git      Git
	writer   Writer
}

type Result struct {
	SkillName     string
	Path          string
	TreeSHA       string
	Changed       bool
	Conflict      bool
	ConflictPaths []string
}

var ErrConflict = errors.New("pull completed with text conflicts")

func NewService(registry Registry, local LocalReader, remote Remote, git Git, writer Writer) *Service {
	return &Service{registry: registry, local: local, remote: remote, git: git, writer: writer}
}

func (s *Service) Pull(ctx context.Context, projectRoot, selector string) (Result, error) {
	installed, err := s.registry.ListProject(ctx, projectRoot)
	if err != nil {
		return Result{}, err
	}
	entry, relative, err := selectSkill(projectRoot, selector, installed)
	if err != nil {
		return Result{}, err
	}
	ref, err := source.ParseRef(entry.SourceRef)
	if err != nil {
		return Result{}, fmt.Errorf("pull ineligible: invalid_source_ref")
	}
	if ref.Kind == source.TagRef {
		return Result{}, fmt.Errorf("pull ineligible: fixed_source_ref")
	}
	if err := workspace.EnsureContained(projectRoot, entry.Path, true); err != nil {
		return Result{}, fmt.Errorf("pull ineligible: unsafe_local_path: %w", err)
	}
	local, err := s.local.Read(entry.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", relative, err)
	}
	if local.Snapshot.HasGeneratedConflictMarker() {
		return Result{}, fmt.Errorf("pull ineligible: unresolved_conflict")
	}
	repository, reason := source.ParseRepository(entry.Repository)
	if reason != "" {
		return Result{}, fmt.Errorf("pull ineligible: %s", reason)
	}
	if err := s.checkTracked(ctx, projectRoot, relative, local.Files); err != nil {
		return Result{}, err
	}

	base, err := s.remote.ReadSkill(ctx, repository, "", entry.TreeSHA)
	if err != nil {
		return Result{}, fmt.Errorf("read sync point: %w", err)
	}
	if err := requireManagedName(base, entry.Name); err != nil {
		return Result{}, fmt.Errorf("read sync point: %w", err)
	}
	current, err := s.remote.ReadSkill(ctx, repository, entry.SourcePath, entry.SourceRef)
	if err != nil {
		return Result{}, fmt.Errorf("read source ref: %w", err)
	}
	if err := requireManagedName(current, entry.Name); err != nil {
		return Result{}, fmt.Errorf("read source ref: %w", err)
	}
	result := Result{SkillName: entry.Name, Path: relative, TreeSHA: current.TreeSHA}
	if workspace.ExactSnapshot(local, current) {
		if current.TreeSHA == entry.TreeSHA && (current.CommitSHA == "" || current.CommitSHA == entry.CommitSHA) {
			return result, nil
		}
		if err := s.registry.Advance(projectRoot, entry.Name, entry.Skill, current.CommitSHA, current.TreeSHA); err != nil {
			return Result{}, fmt.Errorf("advance pulled skill baseline: %w", err)
		}
		result.Changed = true
		return result, nil
	}
	if !workspace.ExactSnapshot(local, base) {
		merged, conflictPaths, err := mergeapp.ThreeWay(ctx, s.git, base, local, current)
		if err != nil {
			return Result{}, err
		}
		if err := s.writer.ReplaceExact(entry.Path, merged, local, func() error {
			return s.registry.Advance(projectRoot, entry.Name, entry.Skill, current.CommitSHA, current.TreeSHA)
		}); err != nil {
			return Result{}, fmt.Errorf("apply merged skill: %w", err)
		}
		result.Changed = true
		result.ConflictPaths = projectConflictPaths(relative, conflictPaths)
		result.Conflict = len(result.ConflictPaths) > 0
		if result.Conflict {
			return result, ErrConflict
		}
		return result, nil
	}
	if err := s.writer.ReplaceExact(entry.Path, current, local, func() error {
		return s.registry.Advance(projectRoot, entry.Name, entry.Skill, current.CommitSHA, current.TreeSHA)
	}); err != nil {
		return Result{}, fmt.Errorf("apply pulled skill: %w", err)
	}
	result.Changed = true
	return result, nil
}

func projectConflictPaths(skillPath string, conflictPaths []string) []string {
	result := make([]string, len(conflictPaths))
	for index, conflictPath := range conflictPaths {
		result[index] = filepath.ToSlash(filepath.Join(skillPath, filepath.FromSlash(conflictPath)))
	}
	return result
}

func (s *Service) checkTracked(
	ctx context.Context,
	projectRoot string,
	relativeSkill string,
	files map[string][]byte,
) error {
	tracked, err := s.git.TrackedFiles(ctx, projectRoot, relativeSkill)
	if err != nil {
		return err
	}
	trackedSet := make(map[string]struct{}, len(tracked))
	for _, path := range tracked {
		trackedSet[filepath.ToSlash(path)] = struct{}{}
	}
	untracked := make([]string, 0)
	for path := range files {
		fullPath := filepath.ToSlash(filepath.Join(relativeSkill, filepath.FromSlash(path)))
		if _, ok := trackedSet[fullPath]; !ok {
			untracked = append(untracked, fullPath)
		}
	}
	if len(untracked) > 0 {
		sort.Strings(untracked)
		return fmt.Errorf("pull requires every skill file to be tracked by the project Git repository; untracked: %s", strings.Join(untracked, ", "))
	}
	return nil
}

func requireManagedName(snapshot source.SkillSnapshot, managedName string) error {
	name, err := skill.ParseName(snapshot.Files["SKILL.md"])
	if err != nil {
		return err
	}
	if name != managedName {
		return fmt.Errorf("source skill name %q does not match managed name %q", name, managedName)
	}
	return nil
}

func selectSkill(projectRoot, selector string, installed []manifest.InstalledSkill) (manifest.InstalledSkill, string, error) {
	if selector == "" {
		return manifest.InstalledSkill{}, "", fmt.Errorf("skill selector is required")
	}
	cleanSelector := filepath.ToSlash(filepath.Clean(selector))
	type candidate struct {
		entry    manifest.InstalledSkill
		relative string
	}
	matches := make([]candidate, 0)
	for _, entry := range installed {
		relative, err := filepath.Rel(filepath.Clean(projectRoot), filepath.Clean(entry.Path))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return manifest.InstalledSkill{}, "", fmt.Errorf("skill path %q is outside project root", entry.Path)
		}
		relative = filepath.ToSlash(relative)
		if entry.Name == selector || relative == cleanSelector {
			matches = append(matches, candidate{entry: entry, relative: relative})
		}
	}
	if len(matches) == 0 {
		return manifest.InstalledSkill{}, "", fmt.Errorf("skill %q was not found in the current project", selector)
	}
	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.relative)
		}
		sort.Strings(paths)
		return manifest.InstalledSkill{}, "", fmt.Errorf("skill name %q is ambiguous; use one of: %s", selector, strings.Join(paths, ", "))
	}
	return matches[0].entry, matches[0].relative, nil
}
