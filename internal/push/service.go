package push

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/skill"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

var (
	ErrRemoteChanged  = errors.New("remote skill changed")
	ErrMetadataUpdate = errors.New("remote push succeeded but local metadata update failed")
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
	ReadPermission(ctx context.Context, repository source.Repository, branch string) (canPush bool, err error)
}

type Inventory interface {
	PushFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error)
}

type Pusher interface {
	PushSkill(
		ctx context.Context,
		repositoryURL string,
		branch string,
		skillPath string,
		expectedTreeSHA string,
		snapshot source.SkillSnapshot,
		message string,
	) (gitcli.PushResult, error)
}

type Service struct {
	registry  Registry
	local     LocalReader
	remote    Remote
	inventory Inventory
	pusher    Pusher
}

type Result struct {
	SkillName string
	Path      string
	TreeSHA   string
	Pushed    bool
}

func NewService(
	registry Registry,
	local LocalReader,
	remote Remote,
	inventory Inventory,
	pusher Pusher,
) *Service {
	return &Service{
		registry: registry, local: local, remote: remote,
		inventory: inventory, pusher: pusher,
	}
}

func (s *Service) Push(ctx context.Context, projectRoot, selector string) (Result, error) {
	installed, err := s.registry.ListProject(ctx, projectRoot)
	if err != nil {
		return Result{}, err
	}
	entry, relative, err := selectSkill(projectRoot, selector, installed)
	if err != nil {
		return Result{}, err
	}
	ref, err := source.ParseRef(entry.SourceRef)
	if err != nil || ref.Kind != source.BranchRef {
		return Result{}, fmt.Errorf("push ineligible: source_ref_read_only")
	}
	if err := workspace.EnsureContained(projectRoot, entry.Path, true); err != nil {
		return Result{}, fmt.Errorf("push ineligible: unsafe_local_path: %w", err)
	}
	local, err := s.local.Read(entry.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", relative, err)
	}
	if local.Snapshot.HasGeneratedConflictMarker() {
		return Result{}, fmt.Errorf("push ineligible: unresolved_conflict")
	}
	localName, err := skill.ParseName(local.Files["SKILL.md"])
	if err != nil {
		return Result{}, fmt.Errorf("push ineligible: invalid_local_skill: %w", err)
	}
	if localName != entry.Name {
		return Result{}, fmt.Errorf("push ineligible: invalid_local_skill: name %q does not match managed name %q", localName, entry.Name)
	}
	repository, reason := source.ParseRepository(entry.Repository)
	if reason != "" {
		return Result{}, fmt.Errorf("push ineligible: %s", reason)
	}
	if err := s.checkPushFiles(ctx, projectRoot, relative, local); err != nil {
		return Result{}, err
	}
	canPush, err := s.remote.ReadPermission(ctx, repository, ref.Name)
	if err != nil {
		return Result{}, fmt.Errorf("push eligibility unknown: permission_unknown: %w", err)
	}
	if !canPush {
		return Result{}, fmt.Errorf("push ineligible: repository_read_only")
	}
	current, err := s.remote.ReadSkill(ctx, repository, entry.SourcePath, entry.SourceRef)
	if err != nil {
		return Result{}, fmt.Errorf("read source ref: %w", err)
	}
	if current.TreeSHA != entry.TreeSHA {
		return Result{}, fmt.Errorf("%w: pull before push (local sync point %s, remote %s)", ErrRemoteChanged, entry.TreeSHA, current.TreeSHA)
	}
	if workspace.ExactSnapshot(local, current) {
		if current.CommitSHA != "" && current.CommitSHA != entry.CommitSHA {
			if err := s.registry.Advance(projectRoot, entry.Name, entry.Skill, current.CommitSHA, current.TreeSHA); err != nil {
				return Result{}, fmt.Errorf("advance push baseline: %w", err)
			}
		}
		return Result{SkillName: entry.Name, Path: relative, TreeSHA: current.TreeSHA}, nil
	}

	snapshot := sourceSnapshot(local)
	pushResult, err := s.pusher.PushSkill(
		ctx,
		entry.Repository,
		ref.Name,
		entry.SourcePath,
		entry.TreeSHA,
		snapshot,
		"chore(skill): sync "+entry.Name,
	)
	if err != nil {
		if errors.Is(err, gitcli.ErrRemoteChanged) {
			return Result{}, fmt.Errorf("%w: source changed during push; pull before retrying", ErrRemoteChanged)
		}
		return Result{}, err
	}
	result := Result{SkillName: entry.Name, Path: relative, TreeSHA: pushResult.TreeSHA, Pushed: pushResult.Pushed}
	if pushResult.TreeSHA != entry.TreeSHA || pushResult.CommitSHA != entry.CommitSHA {
		if err := s.registry.Advance(projectRoot, entry.Name, entry.Skill, pushResult.CommitSHA, pushResult.TreeSHA); err != nil {
			return result, fmt.Errorf("%w: %v; do not retry push; run pull to reconcile the local baseline", ErrMetadataUpdate, err)
		}
	}
	return result, nil
}

func (s *Service) checkPushFiles(ctx context.Context, root, relative string, local workspace.LocalSkill) error {
	allowed, err := s.inventory.PushFiles(ctx, root, relative)
	if err != nil {
		return err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, filePath := range allowed {
		allowedSet[filepath.ToSlash(filePath)] = struct{}{}
	}
	ignored := make([]string, 0)
	for filePath := range local.Files {
		fullPath := filepath.ToSlash(filepath.Join(relative, filepath.FromSlash(filePath)))
		if _, ok := allowedSet[fullPath]; !ok {
			ignored = append(ignored, fullPath)
		}
	}
	if len(ignored) > 0 {
		sort.Strings(ignored)
		return fmt.Errorf("push ineligible: ignored_files: %s", strings.Join(ignored, ", "))
	}
	return nil
}

func sourceSnapshot(local workspace.LocalSkill) source.SkillSnapshot {
	files := make(map[string][]byte, len(local.Files))
	for filePath, content := range local.Files {
		files[filePath] = append([]byte(nil), content...)
	}
	return source.SkillSnapshot{Files: files, Executable: local.Executable}
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
