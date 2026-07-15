package install

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/discovery"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/skill"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type Remote interface {
	ResolveSourceRef(ctx context.Context, repository source.Repository, ref string) (source.ResolvedRef, error)
	ReadSkill(ctx context.Context, repository source.Repository, skillPath, revision string) (source.SkillSnapshot, error)
	DiscoverSkills(ctx context.Context, repository source.Repository, revision string) (discovery.Result, error)
}

type Store interface {
	Read(projectRoot string) (manifest.Document, error)
	Write(projectRoot string, document manifest.Document) error
}

type Writer interface {
	Install(path string, snapshot source.SkillSnapshot) error
	ReplaceExact(path string, snapshot source.SkillSnapshot, expected workspace.LocalSkill, commit func() error) error
}

type Service struct {
	remote Remote
	store  Store
	writer Writer
}

type Result struct {
	Name           string
	Path           string
	Installed      bool
	Repinned       bool
	PreviousRef    string
	PreviousRefSHA string
	SourceRef      string
	RefSHA         string
}

type Options struct {
	AcceptMovedTag bool
}

type plan struct {
	result   Result
	entry    manifest.Skill
	snapshot source.SkillSnapshot
	target   string
	noOp     bool
	repin    bool
	previous manifest.Skill
}

func NewService(remote Remote, store Store, writer Writer) *Service {
	return &Service{remote: remote, store: store, writer: writer}
}

func (s *Service) Install(
	ctx context.Context,
	projectRoot, repositoryInput, sourcePath string,
	ref source.Ref,
	options Options,
) (Result, error) {
	repository, repositoryURL, err := parseRepository(repositoryInput)
	if err != nil {
		return Result{}, err
	}
	resolved, err := s.resolveRef(ctx, repository, ref)
	if err != nil {
		return Result{}, err
	}
	revision := resolved.CommitSHA
	if sourcePath == "SKILL.md" {
		return Result{}, fmt.Errorf("repository-root SKILL.md is not supported")
	}
	if !discovery.LooksLikePath(sourcePath) {
		found, err := s.remote.DiscoverSkills(ctx, repository, revision)
		if err != nil {
			return Result{}, fmt.Errorf("discover source skills: %w", err)
		}
		candidate, err := discovery.Resolve(found.Skills, sourcePath)
		if err != nil {
			return Result{}, err
		}
		sourcePath = candidate.Path
		revision = found.CommitSHA
	}
	return s.installPath(ctx, projectRoot, repository, repositoryURL, sourcePath, ref, resolved, revision, options)
}

func (s *Service) Discover(ctx context.Context, repositoryInput string, ref source.Ref) (discovery.Result, error) {
	repository, _, err := parseRepository(repositoryInput)
	if err != nil {
		return discovery.Result{}, err
	}
	resolved, err := s.resolveRef(ctx, repository, ref)
	if err != nil {
		return discovery.Result{}, err
	}
	result, err := s.remote.DiscoverSkills(ctx, repository, resolved.CommitSHA)
	if err != nil {
		return discovery.Result{}, fmt.Errorf("discover source skills: %w", err)
	}
	if len(result.Skills) == 0 {
		return discovery.Result{}, fmt.Errorf("no skills found in %s", repositoryInput)
	}
	return result, nil
}

func (s *Service) InstallAll(ctx context.Context, projectRoot, repositoryInput string, ref source.Ref) ([]Result, error) {
	repository, repositoryURL, err := parseRepository(repositoryInput)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRef(ctx, repository, ref)
	if err != nil {
		return nil, err
	}
	found, err := s.remote.DiscoverSkills(ctx, repository, resolved.CommitSHA)
	if err != nil {
		return nil, fmt.Errorf("discover source skills: %w", err)
	}
	if len(found.Skills) == 0 {
		return nil, fmt.Errorf("no skills found in %s", repositoryInput)
	}
	if err := discovery.FindNameCollisions(found.Skills); err != nil {
		return nil, err
	}
	document, err := s.store.Read(projectRoot)
	if err != nil {
		return nil, err
	}
	plans := make([]plan, 0, len(found.Skills))
	for _, candidate := range found.Skills {
		snapshot, err := s.remote.ReadSkill(ctx, repository, candidate.Path, found.CommitSHA)
		if err != nil {
			return nil, fmt.Errorf("read source skill %s: %w", candidate.DisplayName(), err)
		}
		if snapshot.CommitSHA != found.CommitSHA || snapshot.TreeSHA != candidate.TreeSHA {
			return nil, fmt.Errorf("source skill %s changed during discovery", candidate.DisplayName())
		}
		planned, err := s.planInstall(
			projectRoot, repositoryURL, candidate.Path, ref, resolved, snapshot, document, Options{}, false,
		)
		if err != nil {
			return nil, fmt.Errorf("preflight %s: %w", candidate.DisplayName(), err)
		}
		plans = append(plans, planned)
	}

	results := make([]Result, 0, len(plans))
	for _, planned := range plans {
		result, err := s.executePlan(ctx, repository, projectRoot, planned, &document)
		if err != nil {
			return results, fmt.Errorf("install %s: %w", planned.result.Name, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) installPath(
	ctx context.Context,
	projectRoot string,
	repository source.Repository,
	repositoryURL, sourcePath string,
	ref source.Ref,
	resolved source.ResolvedRef,
	revision string,
	options Options,
) (Result, error) {
	sourcePath, err := normalizeSourcePath(sourcePath)
	if err != nil {
		return Result{}, err
	}
	document, err := s.store.Read(projectRoot)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := s.remote.ReadSkill(ctx, repository, sourcePath, revision)
	if err != nil {
		return Result{}, fmt.Errorf("read source skill: %w", err)
	}
	planned, err := s.planInstall(
		projectRoot, repositoryURL, sourcePath, ref, resolved, snapshot, document, options, true,
	)
	if err != nil {
		return Result{}, err
	}
	return s.executePlan(ctx, repository, projectRoot, planned, &document)
}

func (s *Service) planInstall(
	projectRoot, repositoryURL, sourcePath string,
	ref source.Ref,
	resolved source.ResolvedRef,
	snapshot source.SkillSnapshot,
	document manifest.Document,
	options Options,
	allowRepin bool,
) (plan, error) {
	if resolved.RefSHA == "" || resolved.CommitSHA == "" || snapshot.CommitSHA == "" || snapshot.TreeSHA == "" {
		return plan{}, fmt.Errorf("source snapshot is missing commit or tree SHA")
	}
	if snapshot.CommitSHA != resolved.CommitSHA {
		return plan{}, fmt.Errorf("source snapshot changed during ref resolution")
	}
	if err := workspace.ValidateSnapshot(snapshot); err != nil {
		return plan{}, err
	}
	skillDocument, ok := snapshot.Files["SKILL.md"]
	if !ok {
		return plan{}, fmt.Errorf("source path has no SKILL.md")
	}
	name, err := skill.ParseName(skillDocument)
	if err != nil {
		return plan{}, err
	}
	if path.Base(sourcePath) != name {
		return plan{}, fmt.Errorf("source directory %q does not match skill name %q", path.Base(sourcePath), name)
	}
	relativeDestination := filepath.ToSlash(filepath.Join(".agents", "skills", name))
	result := Result{Name: name, Path: relativeDestination, SourceRef: ref.FullName, RefSHA: resolved.RefSHA}
	entry := manifest.Skill{
		Repository: repositoryURL, SourcePath: sourcePath, SourceRef: ref.FullName,
		RefSHA: resolved.RefSHA, CommitSHA: resolved.CommitSHA,
		TreeSHA: snapshot.TreeSHA, Destination: relativeDestination,
	}
	if existing, found := document.Skills[name]; found {
		if existing == entry {
			local, err := workspace.ReadSkill(filepath.Join(projectRoot, filepath.FromSlash(relativeDestination)))
			if err != nil || !workspace.ExactSnapshot(local, snapshot) {
				return plan{}, fmt.Errorf("managed skill %q does not match its recorded snapshot; run status", name)
			}
			return plan{result: result, entry: entry, snapshot: snapshot, noOp: true}, nil
		}
		if existing.Repository != repositoryURL || existing.SourcePath != sourcePath || existing.Destination != relativeDestination {
			return plan{}, fmt.Errorf("managed skill %q already has a different source", name)
		}
		existingRef, err := source.ParseRef(existing.SourceRef)
		if err != nil || existingRef.Kind != source.TagRef || ref.Kind != source.TagRef {
			return plan{}, fmt.Errorf("managed skill %q cannot change between branch and tag sources", name)
		}
		if options.AcceptMovedTag && existing.SourceRef != entry.SourceRef {
			return plan{}, fmt.Errorf("--accept-moved-tag is only valid when reinstalling the same tag")
		}
		if !allowRepin {
			return plan{}, fmt.Errorf("managed skill %q tag cannot be changed with --all", name)
		}
		if existing.SourceRef == entry.SourceRef && existing.RefSHA != entry.RefSHA && !options.AcceptMovedTag {
			return plan{}, fmt.Errorf("tag_moved: %s changed from %s to %s; rerun with --accept-moved-tag", ref.Name, existing.RefSHA, entry.RefSHA)
		}
		if existing.SourceRef == entry.SourceRef && existing.RefSHA == entry.RefSHA {
			return plan{}, fmt.Errorf("managed skill %q source ref is unchanged but its snapshot differs", name)
		}
		result.PreviousRef = existing.SourceRef
		result.PreviousRefSHA = existing.RefSHA
		return plan{
			result: result, entry: entry, snapshot: snapshot, target: filepath.Join(projectRoot, filepath.FromSlash(relativeDestination)),
			repin: true, previous: existing,
		}, nil
	}
	if options.AcceptMovedTag {
		return plan{}, fmt.Errorf("--accept-moved-tag is only valid for an installed skill whose tag moved")
	}
	for existingName, existing := range document.Skills {
		if existing.Repository == repositoryURL && existing.SourcePath == sourcePath && existing.SourceRef == ref.FullName {
			return plan{}, fmt.Errorf("source is already managed as %q", existingName)
		}
		if existing.Destination == relativeDestination {
			return plan{}, fmt.Errorf("destination is already managed as %q", existingName)
		}
	}

	target := filepath.Join(projectRoot, filepath.FromSlash(relativeDestination))
	if err := workspace.EnsureContained(projectRoot, target, true); err != nil {
		return plan{}, err
	}
	if _, err := os.Lstat(target); err == nil {
		return plan{}, fmt.Errorf("install destination already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return plan{}, fmt.Errorf("inspect install destination: %w", err)
	}
	return plan{result: result, entry: entry, snapshot: snapshot, target: target}, nil
}

func (s *Service) executePlan(
	ctx context.Context,
	repository source.Repository,
	projectRoot string,
	planned plan,
	document *manifest.Document,
) (Result, error) {
	if planned.noOp {
		return planned.result, nil
	}
	if planned.repin {
		base, err := s.remote.ReadSkill(ctx, repository, "", planned.previous.TreeSHA)
		if err != nil {
			return Result{}, fmt.Errorf("read recorded tag baseline: %w", err)
		}
		local, err := workspace.ReadSkill(planned.target)
		if err != nil || !workspace.ExactSnapshot(local, base) {
			return Result{}, fmt.Errorf("managed skill %q has local changes; restore its recorded snapshot before changing tags", planned.result.Name)
		}
		commit := func() error {
			document.Skills[planned.result.Name] = planned.entry
			if err := s.store.Write(projectRoot, *document); err != nil {
				document.Skills[planned.result.Name] = planned.previous
				return err
			}
			return nil
		}
		if planned.previous.TreeSHA == planned.entry.TreeSHA {
			if err := commit(); err != nil {
				return Result{}, fmt.Errorf("write management file: %w", err)
			}
		} else if err := s.writer.ReplaceExact(planned.target, planned.snapshot, local, commit); err != nil {
			return Result{}, err
		}
		planned.result.Repinned = true
		return planned.result, nil
	}
	if err := s.writer.Install(planned.target, planned.snapshot); err != nil {
		return Result{}, err
	}
	document.Skills[planned.result.Name] = planned.entry
	if err := s.store.Write(projectRoot, *document); err != nil {
		if rollbackErr := os.RemoveAll(planned.target); rollbackErr != nil {
			return Result{}, fmt.Errorf("write management file: %w; rollback failed: %v", err, rollbackErr)
		}
		delete(document.Skills, planned.result.Name)
		return Result{}, fmt.Errorf("write management file: %w", err)
	}
	planned.result.Installed = true
	return planned.result, nil
}

func (s *Service) resolveRef(ctx context.Context, repository source.Repository, ref source.Ref) (source.ResolvedRef, error) {
	parsed, err := source.ParseRef(ref.FullName)
	if err != nil || parsed != ref {
		return source.ResolvedRef{}, fmt.Errorf("invalid source ref %q", ref.FullName)
	}
	resolved, err := s.remote.ResolveSourceRef(ctx, repository, ref.FullName)
	if err != nil {
		return source.ResolvedRef{}, fmt.Errorf("resolve source ref %s: %w", ref.FullName, err)
	}
	if resolved.RefSHA == "" || resolved.CommitSHA == "" {
		return source.ResolvedRef{}, fmt.Errorf("resolve source ref %s: incomplete result", ref.FullName)
	}
	return resolved, nil
}

func normalizeSourcePath(value string) (string, error) {
	if path.Base(value) != "SKILL.md" {
		return value, nil
	}
	directory := path.Dir(value)
	if directory == "." {
		return "", fmt.Errorf("repository-root SKILL.md is not supported")
	}
	return directory, nil
}

func parseRepository(value string) (source.Repository, string, error) {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSpace(value), ".git"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(value, "\\?#\x00\r\n") {
		return source.Repository{}, "", fmt.Errorf("repository must be OWNER/REPO")
	}
	url := "https://github.com/" + parts[0] + "/" + parts[1] + ".git"
	repository, reason := source.ParseRepository(url)
	if reason != "" {
		return source.Repository{}, "", fmt.Errorf("invalid repository: %s", reason)
	}
	return repository, url, nil
}
