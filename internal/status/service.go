package status

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/skill"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/syncstate"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type Eligibility string

const (
	Eligible   Eligibility = "eligible"
	Ineligible Eligibility = "ineligible"
	Unknown    Eligibility = "unknown"
)

type Record struct {
	SkillName       string            `json:"skillName"`
	Path            string            `json:"path"`
	SourceURL       *string           `json:"sourceURL"`
	SourceRef       *string           `json:"sourceRef"`
	State           *syncstate.State  `json:"state"`
	PullEligibility Eligibility       `json:"pullEligibility"`
	PullReason      *string           `json:"pullReason"`
	PushEligibility Eligibility       `json:"pushEligibility"`
	PushReason      *string           `json:"pushReason"`
	Proposal        *proposal.Summary `json:"proposal,omitempty"`
}

type Repository = source.Repository

type PreflightRequest struct {
	Repository     Repository
	Refs           []string
	ReadPermission bool
}

type PreflightRefResult struct {
	Resolved source.ResolvedRef
	Err      error
}

type PreflightResult struct {
	Refs              map[string]PreflightRefResult
	CanPush           bool
	PermissionErr     error
	PermissionChecked bool
}

type Lister interface {
	ListProject(ctx context.Context, projectRoot string) ([]manifest.InstalledSkill, error)
}

type LocalReader interface {
	Read(path string) (workspace.LocalSkill, error)
}

type Remote interface {
	ReadStatusPreflight(ctx context.Context, requests []PreflightRequest) map[string]PreflightResult
	ReadRepositoryTree(ctx context.Context, repository Repository, revision string) (source.RepositoryTree, error)
	ReadSkill(ctx context.Context, repository Repository, skillPath string, revision string) (source.SkillSnapshot, error)
	ListPullRequests(
		ctx context.Context, repository Repository, options proposal.ListOptions,
	) ([]proposal.PullRequest, error)
}

type Inventory interface {
	TrackedFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error)
	PushFiles(ctx context.Context, projectRoot, relativePath string) ([]string, error)
}

type Service struct {
	lister    Lister
	local     LocalReader
	remote    Remote
	inventory Inventory
}

func NewService(lister Lister, local LocalReader, remote Remote, inventory ...Inventory) *Service {
	service := &Service{lister: lister, local: local, remote: remote}
	if len(inventory) > 0 {
		service.inventory = inventory[0]
	}
	return service
}

func (s *Service) Inspect(ctx context.Context, projectRoot string) ([]Record, error) {
	installed, err := s.lister.ListProject(ctx, projectRoot)
	if err != nil {
		return nil, err
	}

	inventory := s.readWorkspaceInventory(ctx, projectRoot)
	records := make([]Record, 0, len(installed))
	prepared := make([]preparedSkill, 0, len(installed))
	for _, entry := range installed {
		path, err := projectRelativePath(projectRoot, entry.Path)
		if err != nil {
			return nil, err
		}
		var local workspace.LocalSkill
		if err = workspace.EnsureContained(projectRoot, entry.Path, true); err == nil {
			local, err = s.local.Read(entry.Path)
		}
		if err != nil {
			record := Record{SkillName: entry.Name, Path: path}
			record.SourceURL = pointer(entry.Repository)
			record.SourceRef = pointer(entry.SourceRef)
			reason := "invalid_local_skill"
			switch {
			case errors.Is(err, workspace.ErrUnsafePath):
				reason = "unsafe_local_path"
			case errors.Is(err, workspace.ErrUnsupportedFile):
				reason = "unsupported_local_file"
			}
			record.PullEligibility = Ineligible
			record.PullReason = pointer(reason)
			record.PushEligibility = Ineligible
			record.PushReason = pointer(reason)
			records = append(records, record)
			continue
		}
		record := Record{SkillName: entry.Name, Path: path}
		record.SourceURL = pointer(entry.Repository)
		record.SourceRef = pointer(entry.SourceRef)

		repository, reason := source.ParseRepository(entry.Repository)
		if reason != "" {
			record.PullEligibility = Ineligible
			record.PullReason = pointer(reason)
			record.PushEligibility = Ineligible
			record.PushReason = pointer(reason)
			records = append(records, record)
			continue
		}
		ref, refErr := source.ParseRef(entry.SourceRef)
		if refErr != nil {
			markRemoteUnknown(&record, "source_unavailable")
			records = append(records, record)
			continue
		}
		if local.Snapshot.HasGeneratedConflictMarker() {
			state := syncstate.Conflict
			record.State = &state
			record.PullEligibility = Ineligible
			record.PullReason = pointer("unresolved_conflict")
			record.PushEligibility = Ineligible
			record.PushReason = pointer("unresolved_conflict")
			records = append(records, record)
			continue
		}
		localName, localDocumentErr := skill.ParseName(local.Files["SKILL.md"])
		if localDocumentErr == nil && localName != entry.Name {
			localDocumentErr = fmt.Errorf("local skill name %q does not match managed name %q", localName, entry.Name)
		}
		pullSafety, pushSafety := workspaceSafety(inventory, path, local)
		localTreeSHA, localTreeErr := workspace.TreeSHA(local.Files, local.Executable)
		if localTreeErr != nil {
			record.PullEligibility = Ineligible
			record.PullReason = pointer("invalid_local_skill")
			record.PushEligibility = Ineligible
			record.PushReason = pointer("invalid_local_skill")
			records = append(records, record)
			continue
		}

		prepared = append(prepared, preparedSkill{
			entry:            entry,
			record:           record,
			repository:       repository,
			ref:              ref,
			localTreeSHA:     localTreeSHA,
			localDocumentErr: localDocumentErr,
			pullSafety:       pullSafety,
			pushSafety:       pushSafety,
		})
	}

	preflight := make(map[string]PreflightResult)
	requests := buildPreflightRequests(prepared)
	if len(requests) > 0 {
		preflight = s.remote.ReadStatusPreflight(ctx, requests)
	}
	proposalResults := s.readProposals(ctx, prepared)
	treeCache := make(map[repositoryRevision]treeResult)
	for _, skill := range prepared {
		entry := skill.entry
		record := skill.record
		result, ok := preflight[repositoryKey(skill.repository)]
		if !ok {
			markRemoteUnknown(&record, "source_unavailable")
			records = append(records, record)
			continue
		}
		refResult, ok := result.Refs[entry.SourceRef]
		if !ok || refResult.Err != nil {
			markRemoteUnknown(&record, "source_unavailable")
			records = append(records, record)
			continue
		}
		resolved := refResult.Resolved
		remoteChanged := false
		currentTreeSHA := entry.TreeSHA
		if resolved.CommitSHA != entry.CommitSHA {
			tree, treeErr := s.repositoryTree(ctx, treeCache, skill.repository, resolved.CommitSHA)
			if treeErr != nil {
				markRemoteUnknown(&record, "source_unavailable")
				records = append(records, record)
				continue
			}
			currentTreeSHA, treeErr = skillTreeSHA(tree, entry.SourcePath)
			if treeErr != nil {
				markRemoteUnknown(&record, "source_unavailable")
				records = append(records, record)
				continue
			}
			remoteChanged = currentTreeSHA != entry.TreeSHA
			if remoteChanged {
				current, readErr := s.remote.ReadSkill(ctx, skill.repository, entry.SourcePath, resolved.CommitSHA)
				if readErr != nil || current.TreeSHA != currentTreeSHA || !snapshotHasManagedName(current, entry.Name) {
					markRemoteUnknown(&record, "source_unavailable")
					records = append(records, record)
					continue
				}
			}
		}
		localChanged := skill.localTreeSHA != entry.TreeSHA
		if remoteChanged && skill.localTreeSHA == currentTreeSHA {
			localChanged = false
		}
		state := syncstate.CalculateChanges(
			localChanged,
			remoteChanged,
			false,
		)
		record.State = &state
		tagMoved := skill.ref.Kind == source.TagRef && resolved.RefSHA != entry.RefSHA
		if skill.ref.Kind == source.TagRef {
			record.PullEligibility = Ineligible
			if tagMoved {
				record.PullReason = pointer("tag_moved")
			} else {
				record.PullReason = pointer("fixed_source_ref")
			}
			record.PushEligibility = Ineligible
			record.PushReason = pointer("source_ref_read_only")
			records = append(records, record)
			continue
		}
		if skill.pullSafety == "" {
			record.PullEligibility = Eligible
		} else {
			record.PullEligibility = Ineligible
			record.PullReason = pointer(skill.pullSafety)
		}

		if skill.localDocumentErr != nil {
			record.PushEligibility = Ineligible
			record.PushReason = pointer("invalid_local_skill")
		} else {
			if !result.PermissionChecked || result.PermissionErr != nil {
				record.PushEligibility = Unknown
				record.PushReason = pointer("permission_unknown")
			} else if !result.CanPush {
				record.PushEligibility = Ineligible
				record.PushReason = pointer("repository_read_only")
			} else {
				record.PushEligibility = Eligible
			}
		}
		if record.PushEligibility == Eligible && remoteChanged {
			record.PushEligibility = Ineligible
			record.PushReason = pointer("remote_changed")
		}
		if record.PushEligibility == Eligible && skill.pushSafety != "" {
			record.PushEligibility = Ineligible
			record.PushReason = pointer(skill.pushSafety)
		}
		proposalResult := proposalResults[repositoryKey(skill.repository)]
		if proposalResult.err != nil {
			record.Proposal = &proposal.Summary{State: proposal.Unknown}
			if record.PushEligibility == Eligible {
				record.PushEligibility = Unknown
				record.PushReason = pointer("proposal_unknown")
			}
		} else if summary, found := proposal.Summarize(
			proposalResult.pulls, skill.repository, skill.ref.Name, entry.Name, entry.SourcePath,
			skill.localTreeSHA, currentTreeSHA,
		); found {
			record.Proposal = &summary
			if record.PushEligibility == Eligible {
				record.PushEligibility = Ineligible
				record.PushReason = pointer("open_proposal")
			}
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].Path < records[j].Path })
	return records, nil
}

type proposalReadResult struct {
	pulls []proposal.PullRequest
	err   error
}

func (s *Service) readProposals(ctx context.Context, skills []preparedSkill) map[string]proposalReadResult {
	repositories := make(map[string]Repository)
	for _, skill := range skills {
		if skill.ref.Kind == source.BranchRef {
			repositories[repositoryKey(skill.repository)] = skill.repository
		}
	}
	keys := make([]string, 0, len(repositories))
	for key := range repositories {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make(map[string]proposalReadResult, len(keys))
	var mutex sync.Mutex
	var wait sync.WaitGroup
	limit := make(chan struct{}, 8)
	for _, key := range keys {
		key := key
		wait.Add(1)
		go func() {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			pulls, err := s.remote.ListPullRequests(
				ctx, repositories[key], proposal.ListOptions{State: "open"},
			)
			mutex.Lock()
			results[key] = proposalReadResult{pulls: pulls, err: err}
			mutex.Unlock()
		}()
	}
	wait.Wait()
	return results
}

type repositoryRevision struct {
	repository string
	revision   string
}

type treeResult struct {
	tree source.RepositoryTree
	err  error
}

type preparedSkill struct {
	entry            manifest.InstalledSkill
	record           Record
	repository       Repository
	ref              source.Ref
	localTreeSHA     string
	localDocumentErr error
	pullSafety       string
	pushSafety       string
}

func buildPreflightRequests(skills []preparedSkill) []PreflightRequest {
	type requestBuilder struct {
		repository     Repository
		refs           map[string]struct{}
		readPermission bool
	}
	builders := make(map[string]*requestBuilder)
	for _, skill := range skills {
		key := repositoryKey(skill.repository)
		builder, ok := builders[key]
		if !ok {
			builder = &requestBuilder{repository: skill.repository, refs: make(map[string]struct{})}
			builders[key] = builder
		}
		builder.refs[skill.entry.SourceRef] = struct{}{}
		if skill.ref.Kind == source.BranchRef && skill.localDocumentErr == nil {
			builder.readPermission = true
		}
	}
	keys := make([]string, 0, len(builders))
	for key := range builders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	requests := make([]PreflightRequest, 0, len(keys))
	for _, key := range keys {
		builder := builders[key]
		refs := make([]string, 0, len(builder.refs))
		for ref := range builder.refs {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		requests = append(requests, PreflightRequest{
			Repository:     builder.repository,
			Refs:           refs,
			ReadPermission: builder.readPermission,
		})
	}
	return requests
}

func (s *Service) repositoryTree(
	ctx context.Context,
	cache map[repositoryRevision]treeResult,
	repository Repository,
	revision string,
) (source.RepositoryTree, error) {
	key := repositoryRevision{repository: repositoryKey(repository), revision: revision}
	if result, ok := cache[key]; ok {
		return result.tree, result.err
	}
	tree, err := s.remote.ReadRepositoryTree(ctx, repository, revision)
	cache[key] = treeResult{tree: tree, err: err}
	return tree, err
}

func repositoryKey(repository Repository) string {
	return repository.Owner + "/" + repository.Name
}

func skillTreeSHA(tree source.RepositoryTree, skillPath string) (string, error) {
	treeSHA := tree.SHA
	foundPath := skillPath == ""
	prefix := ""
	if skillPath != "" {
		prefix = skillPath + "/"
		for _, entry := range tree.Entries {
			if entry.Path == skillPath && entry.Type == "tree" {
				foundPath = true
				treeSHA = entry.SHA
				break
			}
		}
	}
	if !foundPath || treeSHA == "" {
		return "", fmt.Errorf("skill path %q does not exist", skillPath)
	}
	foundDocument := false
	for _, entry := range tree.Entries {
		relative := entry.Path
		if skillPath != "" {
			if !strings.HasPrefix(entry.Path, prefix) {
				continue
			}
			relative = strings.TrimPrefix(entry.Path, prefix)
		}
		if entry.Type == "tree" {
			continue
		}
		if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
			return "", fmt.Errorf("unsupported remote entry %s", entry.Path)
		}
		if relative == "SKILL.md" {
			foundDocument = true
		}
	}
	if !foundDocument {
		return "", fmt.Errorf("skill path %q has no SKILL.md", skillPath)
	}
	return treeSHA, nil
}

type workspaceInventory struct {
	enabled    bool
	tracked    map[string]struct{}
	push       map[string]struct{}
	trackedErr error
	pushErr    error
}

func (s *Service) readWorkspaceInventory(ctx context.Context, root string) workspaceInventory {
	if s.inventory == nil {
		return workspaceInventory{}
	}
	tracked, trackedErr := s.inventory.TrackedFiles(ctx, root, ".agents/skills")
	pushFiles, pushErr := s.inventory.PushFiles(ctx, root, ".agents/skills")
	return workspaceInventory{
		enabled:    true,
		tracked:    pathSet(tracked),
		push:       pathSet(pushFiles),
		trackedErr: trackedErr,
		pushErr:    pushErr,
	}
}

func workspaceSafety(
	inventory workspaceInventory,
	relative string,
	local workspace.LocalSkill,
) (pullReason string, pushReason string) {
	if !inventory.enabled {
		return "", ""
	}
	if inventory.trackedErr != nil {
		return "git_inventory_unknown", "git_inventory_unknown"
	}
	if inventory.pushErr != nil {
		return "", "git_inventory_unknown"
	}
	for filePath := range local.Files {
		fullPath := filepath.ToSlash(filepath.Join(relative, filepath.FromSlash(filePath)))
		if _, ok := inventory.tracked[fullPath]; !ok {
			pullReason = "untracked_files"
		}
		if _, ok := inventory.push[fullPath]; !ok {
			pushReason = "ignored_files"
		}
	}
	return pullReason, pushReason
}

func pathSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, value := range paths {
		set[filepath.ToSlash(value)] = struct{}{}
	}
	return set
}

func projectRelativePath(root, path string) (string, error) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("make skill path relative: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("skill path %q is outside project root %q", path, root)
	}
	return filepath.ToSlash(relative), nil
}

func markRemoteUnknown(record *Record, reason string) {
	record.PullEligibility = Unknown
	record.PullReason = pointer(reason)
	record.PushEligibility = Unknown
	record.PushReason = pointer(reason)
}

func pointer(value string) *string {
	return &value
}

func snapshotHasManagedName(snapshot source.SkillSnapshot, managedName string) bool {
	name, err := skill.ParseName(snapshot.Files["SKILL.md"])
	return err == nil && name == managedName
}
