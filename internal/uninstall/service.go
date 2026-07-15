package uninstall

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type Registry interface {
	ListProject(ctx context.Context, projectRoot string) ([]manifest.InstalledSkill, error)
	Remove(projectRoot, name string, expected manifest.Skill) error
}

type LocalReader interface {
	Read(path string) (workspace.LocalSkill, error)
}

type Remover interface {
	Remove(path string, expected *workspace.LocalSkill, commit func() error) error
}

type Service struct {
	registry Registry
	local    LocalReader
	remover  Remover
}

type Options struct {
	Force bool
}

type Result struct {
	Name               string
	Path               string
	DestinationMissing bool
}

func NewService(registry Registry, local LocalReader, remover Remover) *Service {
	return &Service{registry: registry, local: local, remover: remover}
}

func (s *Service) Uninstall(
	ctx context.Context,
	projectRoot, selector string,
	options Options,
) (Result, error) {
	installed, err := s.registry.ListProject(ctx, projectRoot)
	if err != nil {
		return Result{}, err
	}
	entry, relative, err := selectSkill(projectRoot, selector, installed)
	if err != nil {
		return Result{}, err
	}
	result := Result{Name: entry.Name, Path: relative}
	if err := workspace.EnsureContained(projectRoot, entry.Path, true); err != nil {
		return Result{}, fmt.Errorf("uninstall ineligible: unsafe_local_path: %w", err)
	}
	info, err := os.Lstat(entry.Path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := s.registry.Remove(projectRoot, entry.Name, entry.Skill); err != nil {
			return Result{}, fmt.Errorf("remove stale management entry: %w", err)
		}
		result.DestinationMissing = true
		return result, nil
	} else if err != nil {
		return Result{}, fmt.Errorf("inspect %s: %w", relative, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("uninstall ineligible: invalid_local_skill: destination is not a directory: %s", relative)
	}

	var expected *workspace.LocalSkill
	if !options.Force {
		local, err := s.local.Read(entry.Path)
		if err != nil {
			return Result{}, fmt.Errorf("verify local changes before uninstall: %w; rerun with --force to discard the local skill", err)
		}
		treeSHA, err := workspace.TreeSHA(local.Files, local.Executable)
		if err != nil {
			return Result{}, fmt.Errorf("verify local changes before uninstall: %w", err)
		}
		if treeSHA != entry.TreeSHA || len(local.EmptyDirectories) > 0 {
			return Result{}, fmt.Errorf("uninstall refused: %s has local changes; push or preserve them, then retry, or rerun with --force to discard them", relative)
		}
		expected = &local
	}
	if err := s.remover.Remove(entry.Path, expected, func() error {
		return s.registry.Remove(projectRoot, entry.Name, entry.Skill)
	}); err != nil {
		return Result{}, fmt.Errorf("uninstall %s: %w", relative, err)
	}
	return result, nil
}

func selectSkill(
	projectRoot, selector string,
	installed []manifest.InstalledSkill,
) (manifest.InstalledSkill, string, error) {
	if selector == "" {
		return manifest.InstalledSkill{}, "", fmt.Errorf("skill selector is required")
	}
	cleanSelector := filepath.ToSlash(filepath.Clean(selector))
	type candidate struct {
		entry    manifest.InstalledSkill
		relative string
	}
	matches := make([]candidate, 0, 1)
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
