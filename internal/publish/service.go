package publish

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/skill"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

var (
	ErrAlreadyManaged = errors.New("skill is already managed")
	ErrMetadataUpdate = errors.New("remote publish succeeded but local metadata update failed")
)

type Registry interface {
	Read(projectRoot string) (manifest.Document, error)
	Add(projectRoot, name string, expected manifest.Document, skill manifest.Skill) error
}

type LocalReader interface {
	Read(path string) (workspace.LocalSkill, error)
}

type Remote interface {
	ReadPermission(ctx context.Context, repository source.Repository, branch string) (bool, error)
	ReadSkill(
		ctx context.Context, repository source.Repository, skillPath string, revision string,
	) (source.SkillSnapshot, error)
}

type Inventory interface {
	PushFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error)
}

type Publisher interface {
	PublishSkill(
		ctx context.Context,
		repositoryURL string,
		branch string,
		skillPath string,
		snapshot source.SkillSnapshot,
		message string,
	) (gitcli.PushResult, error)
}

type ProposalManager interface {
	FindActive(
		ctx context.Context,
		repository source.Repository,
		baseBranch, skillName, sourcePath string,
	) (*proposal.PullRequest, error)
	FindMerged(
		ctx context.Context,
		repository source.Repository,
		baseBranch, skillName, sourcePath, treeSHA string,
	) (*proposal.PullRequest, error)
	Propose(ctx context.Context, request proposal.Request) (proposal.Result, error)
}

type Service struct {
	registry  Registry
	local     LocalReader
	remote    Remote
	inventory Inventory
	publisher Publisher
	proposals ProposalManager
}

type Result struct {
	SkillName      string
	Path           string
	Repository     string
	SourcePath     string
	CommitSHA      string
	TreeSHA        string
	Published      bool
	Proposed       bool
	ProposalState  string
	ProposalNumber int
	ProposalURL    string
}

func NewService(
	registry Registry,
	local LocalReader,
	remote Remote,
	inventory Inventory,
	publisher Publisher,
	proposals ProposalManager,
) *Service {
	return &Service{
		registry: registry, local: local, remote: remote,
		inventory: inventory, publisher: publisher, proposals: proposals,
	}
}

func (s *Service) Publish(
	ctx context.Context,
	projectRoot, repositoryInput, selector string,
	ref source.Ref,
) (Result, error) {
	return s.publish(ctx, projectRoot, repositoryInput, selector, ref, false)
}

func (s *Service) PublishProposal(
	ctx context.Context,
	projectRoot, repositoryInput, selector string,
	ref source.Ref,
) (Result, error) {
	return s.publish(ctx, projectRoot, repositoryInput, selector, ref, true)
}

func (s *Service) publish(
	ctx context.Context,
	projectRoot, repositoryInput, selector string,
	ref source.Ref,
	pullRequest bool,
) (Result, error) {
	parsedRef, err := source.ParseRef(ref.FullName)
	if err != nil || parsedRef != ref || ref.Kind != source.BranchRef {
		return Result{}, fmt.Errorf("publish requires a valid branch")
	}
	repository, repositoryURL, err := parseRepository(repositoryInput)
	if err != nil {
		return Result{}, err
	}
	document, err := s.registry.Read(projectRoot)
	if err != nil {
		return Result{}, err
	}
	localPath, relativePath, err := resolveLocalSkillPath(projectRoot, selector)
	if err != nil {
		return Result{}, err
	}
	if err := workspace.EnsureContained(projectRoot, localPath, false); err != nil {
		return Result{}, fmt.Errorf("publish ineligible: unsafe_local_path: %w", err)
	}
	local, err := s.local.Read(localPath)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", relativePath, err)
	}
	if local.Snapshot.HasGeneratedConflictMarker() {
		return Result{}, fmt.Errorf("publish ineligible: unresolved_conflict")
	}
	name, err := skill.ParseName(local.Files["SKILL.md"])
	if err != nil {
		return Result{}, fmt.Errorf("publish ineligible: invalid_local_skill: %w", err)
	}
	if path.Base(relativePath) != name {
		return Result{}, fmt.Errorf("publish ineligible: invalid_local_skill: directory %q does not match name %q", path.Base(relativePath), name)
	}
	if existing, found := document.Skills[name]; found {
		return Result{}, fmt.Errorf("%w: %s is linked to %s; use push", ErrAlreadyManaged, name, existing.Repository)
	}

	sourcePath := path.Join("skills", name)
	for existingName, existing := range document.Skills {
		if existing.Repository == repositoryURL && existing.SourcePath == sourcePath && existing.SourceRef == ref.FullName {
			return Result{}, fmt.Errorf("source %s:%s is already managed as %q", repositoryInput, sourcePath, existingName)
		}
	}
	snapshot := sourceSnapshot(local)
	if err := workspace.ValidateSnapshot(snapshot); err != nil {
		return Result{}, fmt.Errorf("publish ineligible: invalid_local_skill: %w", err)
	}
	snapshot.TreeSHA, err = workspace.TreeSHA(snapshot.Files, snapshot.Executable)
	if err != nil {
		return Result{}, fmt.Errorf("publish ineligible: invalid_local_skill: %w", err)
	}
	if err := s.checkPushFiles(ctx, projectRoot, relativePath, local); err != nil {
		return Result{}, err
	}
	canPush, err := s.remote.ReadPermission(ctx, repository, ref.Name)
	if err != nil {
		return Result{}, fmt.Errorf("publish eligibility unknown: permission_unknown: %w", err)
	}
	if !canPush {
		return Result{}, fmt.Errorf("publish ineligible: repository_read_only")
	}
	if pullRequest {
		return s.propose(
			ctx, projectRoot, repositoryInput, repositoryURL, name, relativePath, sourcePath, ref, document, snapshot,
		)
	}
	active, activeErr := s.proposals.FindActive(ctx, repository, ref.Name, name, sourcePath)
	if activeErr != nil {
		return Result{}, fmt.Errorf("publish eligibility unknown: proposal_unknown: %w", activeErr)
	}
	if active != nil {
		return Result{}, fmt.Errorf(
			"publish ineligible: open_proposal: close or merge %s, or rerun with --pr", active.URL,
		)
	}

	pushResult, err := s.publisher.PublishSkill(
		ctx, repositoryURL, ref.Name, sourcePath, snapshot, "feat(skill): publish "+name,
	)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		SkillName: name, Path: relativePath, Repository: repositoryInput, SourcePath: sourcePath,
		CommitSHA: pushResult.CommitSHA, TreeSHA: pushResult.TreeSHA, Published: pushResult.Pushed,
	}
	entry := manifest.Skill{
		Repository: repositoryURL, SourcePath: sourcePath, SourceRef: ref.FullName,
		RefSHA: pushResult.CommitSHA, CommitSHA: pushResult.CommitSHA, TreeSHA: pushResult.TreeSHA,
		Destination: relativePath,
	}
	if err := s.registry.Add(projectRoot, name, document, entry); err != nil {
		return result, fmt.Errorf("%w: %v; rerun publish to link the existing remote skill", ErrMetadataUpdate, err)
	}
	return result, nil
}

func (s *Service) propose(
	ctx context.Context,
	projectRoot, repositoryInput, repositoryURL, name, relativePath, sourcePath string,
	ref source.Ref,
	document manifest.Document,
	snapshot source.SkillSnapshot,
) (Result, error) {
	repository, reason := source.ParseRepository(repositoryURL)
	if reason != "" {
		return Result{}, fmt.Errorf("invalid repository: %s", reason)
	}
	current, err := s.remote.ReadSkill(ctx, repository, sourcePath, ref.FullName)
	if err == nil {
		if current.Exact(snapshot) {
			return s.link(projectRoot, repositoryInput, name, relativePath, sourcePath, ref, document, current)
		}
		merged, mergedErr := s.proposals.FindMerged(
			ctx, repository, ref.Name, name, sourcePath, current.TreeSHA,
		)
		if mergedErr != nil {
			return Result{}, fmt.Errorf("inspect merged proposal: %w", mergedErr)
		}
		if merged != nil {
			return s.link(projectRoot, repositoryInput, name, relativePath, sourcePath, ref, document, current)
		}
		return Result{}, fmt.Errorf(
			"publish ineligible: remote_skill_exists: %s:%s already contains different content",
			repositoryInput, sourcePath,
		)
	} else if !errors.Is(err, source.ErrSkillNotFound) {
		return Result{}, fmt.Errorf("read publish target: %w", err)
	}

	proposed, err := s.proposals.Propose(ctx, proposal.Request{
		Repository: repository, RepositoryURL: repositoryURL, BaseBranch: ref.Name,
		SkillName: name, SourcePath: sourcePath,
		Snapshot: snapshot, Title: "feat(skill): publish " + name,
		Message: "feat(skill): publish " + name,
	})
	if err != nil {
		return Result{}, err
	}
	return publishProposalResult(repositoryInput, name, relativePath, sourcePath, snapshot.TreeSHA, proposed), nil
}

func (s *Service) link(
	projectRoot, repositoryInput, name, relativePath, sourcePath string,
	ref source.Ref,
	document manifest.Document,
	current source.SkillSnapshot,
) (Result, error) {
	result := Result{
		SkillName: name, Path: relativePath, Repository: repositoryInput, SourcePath: sourcePath,
		CommitSHA: current.CommitSHA, TreeSHA: current.TreeSHA,
	}
	entry := manifest.Skill{
		Repository: "https://github.com/" + repositoryInput + ".git",
		SourcePath: sourcePath, SourceRef: ref.FullName,
		RefSHA: current.CommitSHA, CommitSHA: current.CommitSHA, TreeSHA: current.TreeSHA,
		Destination: relativePath,
	}
	if err := s.registry.Add(projectRoot, name, document, entry); err != nil {
		return result, fmt.Errorf("link merged proposal: %w", err)
	}
	return result, nil
}

func publishProposalResult(
	repository, name, relativePath, sourcePath, treeSHA string,
	result proposal.Result,
) Result {
	state := "waiting"
	switch {
	case result.Created:
		state = "created"
	case result.Updated:
		state = "updated"
	case result.Recovered:
		state = "recovered"
	case result.Applied:
		state = "applied"
	case result.Merged:
		state = "merged"
	}
	return Result{
		SkillName: name, Path: relativePath, Repository: repository, SourcePath: sourcePath,
		TreeSHA: treeSHA, Proposed: !result.Applied && !result.Merged, ProposalState: state,
		ProposalNumber: result.PullRequest.Number, ProposalURL: result.PullRequest.URL,
	}
}

func resolveLocalSkillPath(projectRoot, selector string) (string, string, error) {
	if selector == "" || filepath.IsAbs(selector) {
		return "", "", fmt.Errorf("skill selector is required")
	}
	clean := filepath.ToSlash(filepath.Clean(selector))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", fmt.Errorf("invalid local skill selector %q", selector)
	}
	if !strings.Contains(clean, "/") {
		clean = path.Join(".agents", "skills", clean)
	}
	parts := strings.Split(clean, "/")
	if len(parts) != 3 || parts[0] != ".agents" || parts[1] != "skills" || parts[2] == "" {
		return "", "", fmt.Errorf("local skill must be .agents/skills/<name>")
	}
	return filepath.Join(filepath.Clean(projectRoot), filepath.FromSlash(clean)), clean, nil
}

func (s *Service) checkPushFiles(
	ctx context.Context,
	projectRoot, relativePath string,
	local workspace.LocalSkill,
) error {
	allowed, err := s.inventory.PushFiles(ctx, projectRoot, relativePath)
	if err != nil {
		return err
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, filePath := range allowed {
		allowedSet[filepath.ToSlash(filePath)] = struct{}{}
	}
	ignored := make([]string, 0)
	for filePath := range local.Files {
		fullPath := filepath.ToSlash(filepath.Join(relativePath, filepath.FromSlash(filePath)))
		if _, ok := allowedSet[fullPath]; !ok {
			ignored = append(ignored, fullPath)
		}
	}
	if len(ignored) > 0 {
		sort.Strings(ignored)
		return fmt.Errorf("publish ineligible: ignored_files: %s", strings.Join(ignored, ", "))
	}
	return nil
}

func sourceSnapshot(local workspace.LocalSkill) source.SkillSnapshot {
	files := make(map[string][]byte, len(local.Files))
	for filePath, content := range local.Files {
		files[filePath] = append([]byte(nil), content...)
	}
	executable := make(map[string]bool, len(local.Executable))
	for filePath, value := range local.Executable {
		executable[filePath] = value
	}
	return source.SkillSnapshot{Files: files, Executable: executable}
}

func parseRepository(value string) (source.Repository, string, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		parts[0] == "." || parts[0] == ".." || parts[0] == "~" || parts[1] == "." || parts[1] == ".." ||
		strings.TrimSpace(value) != value || strings.HasSuffix(value, ".git") || strings.ContainsAny(value, "\\?#\x00\r\n") {
		return source.Repository{}, "", fmt.Errorf("repository must be OWNER/REPO")
	}
	repositoryURL := "https://github.com/" + parts[0] + "/" + parts[1] + ".git"
	repository, reason := source.ParseRepository(repositoryURL)
	if reason != "" {
		return source.Repository{}, "", fmt.Errorf("invalid repository: %s", reason)
	}
	return repository, repositoryURL, nil
}
