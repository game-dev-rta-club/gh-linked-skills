package proposal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

var (
	ErrAmbiguous = errors.New("multiple active skill proposals")
	ErrDiverged  = errors.New("skill proposal changed outside gh-skill-linker")
	ErrObsolete  = errors.New("skill proposal is obsolete because local matches the source")
)

type Remote interface {
	ListPullRequests(ctx context.Context, repository source.Repository, options ListOptions) ([]PullRequest, error)
	CreatePullRequest(ctx context.Context, repository source.Repository, request CreateRequest) (PullRequest, error)
	UpdatePullRequestBody(ctx context.Context, repository source.Repository, number int, body string) (PullRequest, error)
	ReadSkill(ctx context.Context, repository source.Repository, skillPath, revision string) (source.SkillSnapshot, error)
}

type Git interface {
	FindRef(ctx context.Context, repositoryURL, ref string) (sha string, found bool, err error)
	ProposeSkill(ctx context.Context, request gitcli.ProposalRequest) (gitcli.PushResult, error)
}

type Service struct {
	remote Remote
	git    Git
}

type Request struct {
	Repository    source.Repository
	RepositoryURL string
	BaseBranch    string
	SkillName     string
	SourcePath    string
	BaseTreeSHA   string
	Snapshot      source.SkillSnapshot
	Title         string
	Body          string
	Message       string
}

type Result struct {
	PullRequest PullRequest
	Branch      string
	Created     bool
	Updated     bool
	Waiting     bool
	Recovered   bool
	Applied     bool
	Merged      bool
}

func NewService(remote Remote, git Git) *Service {
	return &Service{remote: remote, git: git}
}

func (service *Service) Propose(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if request.BaseTreeSHA != "" && request.BaseTreeSHA == request.Snapshot.TreeSHA {
		return Result{Applied: true}, nil
	}
	prefix := BranchPrefix(request.SkillName, request.SourcePath)
	pulls, err := service.remote.ListPullRequests(
		ctx, request.Repository, ListOptions{State: "open", Base: request.BaseBranch},
	)
	if err != nil {
		return Result{}, err
	}
	active, err := selectActivePull(pulls, request, prefix)
	if err != nil {
		return Result{}, err
	}
	if active != nil {
		return service.updateActive(ctx, request, *active)
	}
	return service.create(ctx, request, prefix)
}

func (service *Service) updateActive(ctx context.Context, request Request, pull PullRequest) (Result, error) {
	metadata, err := ParseMetadata(pull.Body)
	if err != nil {
		return Result{}, fmt.Errorf("%w: pull request %d has invalid metadata: %v", ErrDiverged, pull.Number, err)
	}
	state, applied := Classify(request.Snapshot.TreeSHA, request.BaseTreeSHA, pull.HeadSHA, metadata)
	if applied {
		return Result{PullRequest: pull, Branch: pull.HeadRef, Applied: true}, nil
	}
	if state == Diverged {
		current, readErr := service.remote.ReadSkill(ctx, request.Repository, request.SourcePath, pull.HeadSHA)
		if readErr != nil || current.TreeSHA != request.Snapshot.TreeSHA {
			return Result{}, fmt.Errorf("%w: pull request %d head no longer matches its metadata", ErrDiverged, pull.Number)
		}
		body, metadataErr := proposalBody(pull.Body, request, current.TreeSHA, pull.HeadSHA)
		if metadataErr != nil {
			return Result{}, metadataErr
		}
		updated, updateErr := service.remote.UpdatePullRequestBody(ctx, request.Repository, pull.Number, body)
		if updateErr != nil {
			return Result{}, updateErr
		}
		return Result{PullRequest: updated, Branch: pull.HeadRef, Recovered: true}, nil
	}
	if state == Obsolete {
		return Result{}, fmt.Errorf("%w: close pull request %s before proposing again", ErrObsolete, pull.URL)
	}
	if state == Waiting {
		return Result{PullRequest: pull, Branch: pull.HeadRef, Waiting: true}, nil
	}
	pushResult, err := service.git.ProposeSkill(ctx, gitcli.ProposalRequest{
		RepositoryURL: request.RepositoryURL, BaseBranch: request.BaseBranch, HeadBranch: pull.HeadRef,
		SkillPath: request.SourcePath, ExpectedBaseTreeSHA: request.BaseTreeSHA,
		ExpectedHeadCommitSHA: pull.HeadSHA, ExpectedProposalTreeSHA: metadata.ProposedTreeSHA,
		MergeBase: state == SourceChanged, Snapshot: request.Snapshot, Message: request.Message,
	})
	if err != nil {
		return Result{}, err
	}
	body, err := proposalBody(pull.Body, request, pushResult.TreeSHA, pushResult.CommitSHA)
	if err != nil {
		return Result{}, err
	}
	updated, err := service.remote.UpdatePullRequestBody(ctx, request.Repository, pull.Number, body)
	if err != nil {
		return Result{}, fmt.Errorf("proposal branch updated but pull request metadata update failed: %w; rerun the same command", err)
	}
	updated.HeadSHA = pushResult.CommitSHA
	updated.Body = body
	return Result{PullRequest: updated, Branch: pull.HeadRef, Updated: true}, nil
}

func (service *Service) create(ctx context.Context, request Request, prefix string) (Result, error) {
	for attempt := 1; attempt <= 100; attempt++ {
		branch := BranchName(prefix, request.BaseTreeSHA, request.Snapshot.TreeSHA, attempt)
		fullRef := "refs/heads/" + branch
		headSHA, found, err := service.git.FindRef(ctx, request.RepositoryURL, fullRef)
		if err != nil {
			return Result{}, err
		}
		if found {
			pulls, listErr := service.remote.ListPullRequests(
				ctx, request.Repository,
				ListOptions{State: "all", Base: request.BaseBranch, Head: request.Repository.Owner + ":" + branch},
			)
			if listErr != nil {
				return Result{}, listErr
			}
			if mergedPull := matchingMergedPull(pulls, request); mergedPull != nil {
				return Result{PullRequest: *mergedPull, Branch: branch, Merged: true}, nil
			}
			if len(pulls) > 0 {
				continue
			}
			current, readErr := service.remote.ReadSkill(ctx, request.Repository, request.SourcePath, headSHA)
			if readErr != nil || current.TreeSHA != request.Snapshot.TreeSHA {
				continue
			}
			return service.createPull(ctx, request, branch, headSHA, true)
		}
		pushResult, pushErr := service.git.ProposeSkill(ctx, gitcli.ProposalRequest{
			RepositoryURL: request.RepositoryURL, BaseBranch: request.BaseBranch, HeadBranch: branch,
			SkillPath: request.SourcePath, ExpectedBaseTreeSHA: request.BaseTreeSHA,
			Snapshot: request.Snapshot, Message: request.Message,
		})
		if pushErr != nil {
			return Result{}, pushErr
		}
		if !pushResult.Pushed {
			return Result{}, fmt.Errorf("proposal branch did not produce a commit")
		}
		return service.createPull(ctx, request, branch, pushResult.CommitSHA, false)
	}
	return Result{}, fmt.Errorf("could not allocate a proposal branch after 100 attempts")
}

func (service *Service) createPull(
	ctx context.Context,
	request Request,
	branch, headSHA string,
	recovered bool,
) (Result, error) {
	body, err := proposalBody(request.Body, request, request.Snapshot.TreeSHA, headSHA)
	if err != nil {
		return Result{}, err
	}
	pull, err := service.remote.CreatePullRequest(ctx, request.Repository, CreateRequest{
		Title: request.Title, Head: branch, Base: request.BaseBranch, Body: body,
	})
	if err != nil {
		return Result{}, fmt.Errorf("proposal branch %s is ready but pull request creation failed: %w; rerun the same command", branch, err)
	}
	return Result{PullRequest: pull, Branch: branch, Created: true, Recovered: recovered}, nil
}

func selectActivePull(pulls []PullRequest, request Request, prefix string) (*PullRequest, error) {
	repository := request.Repository.Owner + "/" + request.Repository.Name
	matches := make([]PullRequest, 0, 1)
	for _, pull := range pulls {
		if pull.State != "open" || pull.HeadRepository != repository || pull.BaseRepository != repository ||
			pull.BaseRef != request.BaseBranch || !strings.HasPrefix(pull.HeadRef, prefix+"/") {
			continue
		}
		metadata, err := ParseMetadata(pull.Body)
		if err != nil {
			return nil, fmt.Errorf("%w: pull request %d uses the proposal branch namespace but has invalid metadata", ErrDiverged, pull.Number)
		}
		if metadata.SourcePath != request.SourcePath || metadata.BaseRef != "refs/heads/"+request.BaseBranch {
			return nil, fmt.Errorf("%w: pull request %d metadata does not match its branch", ErrDiverged, pull.Number)
		}
		matches = append(matches, pull)
	}
	if len(matches) > 1 {
		urls := make([]string, len(matches))
		for index := range matches {
			urls[index] = matches[index].URL
		}
		return nil, fmt.Errorf("%w: %s", ErrAmbiguous, strings.Join(urls, ", "))
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

func matchingMergedPull(pulls []PullRequest, request Request) *PullRequest {
	repository := request.Repository.Owner + "/" + request.Repository.Name
	for index := range pulls {
		pull := &pulls[index]
		if !pull.Merged || pull.HeadRepository != repository || pull.BaseRepository != repository ||
			pull.BaseRef != request.BaseBranch {
			continue
		}
		metadata, err := ParseMetadata(pull.Body)
		if err == nil && metadata.SourcePath == request.SourcePath &&
			metadata.BaseRef == "refs/heads/"+request.BaseBranch &&
			metadata.ProposedTreeSHA == request.Snapshot.TreeSHA {
			return pull
		}
	}
	return nil
}

func proposalBody(existing string, request Request, treeSHA, headSHA string) (string, error) {
	if strings.TrimSpace(existing) == "" {
		existing = "Synchronize `" + request.SkillName + "` from a project."
	}
	return SetMetadata(existing, Metadata{
		Version: MetadataVersion, SourcePath: request.SourcePath, BaseRef: "refs/heads/" + request.BaseBranch,
		BaseTreeSHA: request.BaseTreeSHA, ProposedTreeSHA: treeSHA, HeadCommitSHA: headSHA,
	})
}

func validateRequest(request Request) error {
	if request.Repository.Owner == "" || request.Repository.Name == "" || request.RepositoryURL == "" ||
		request.BaseBranch == "" || request.SkillName == "" || request.SourcePath == "" || request.Title == "" || request.Message == "" {
		return fmt.Errorf("repository, base branch, skill identity, title, and commit message are required")
	}
	if request.Snapshot.TreeSHA == "" {
		return fmt.Errorf("proposal snapshot tree SHA is required")
	}
	return nil
}
