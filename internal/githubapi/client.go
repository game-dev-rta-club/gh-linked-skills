package githubapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/discovery"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/skill"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/status"
)

type Client struct {
	rest        *api.RESTClient
	graphql     GraphQLDoer
	pushProbe   PushProbe
	refResolver RefResolver
	treeMu      sync.Mutex
	trees       map[repositoryRevision]treeResponse
}

type GraphQLDoer interface {
	DoWithContext(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error
}

type repositoryRevision struct {
	owner    string
	name     string
	revision string
}

type PushProbe interface {
	CanPush(ctx context.Context, repositoryURL, branch string) (bool, error)
}

type RefResolver interface {
	ResolveRef(ctx context.Context, repositoryURL, ref string) (string, error)
}

type GitRemote interface {
	PushProbe
	RefResolver
}

type treeResponse struct {
	SHA       string `json:"sha"`
	Truncated bool   `json:"truncated"`
	Entries   []struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
}

type gitObject struct {
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

func New(rest *api.RESTClient, pushProbe ...PushProbe) *Client {
	client := &Client{rest: rest}
	if len(pushProbe) > 0 {
		client.pushProbe = pushProbe[0]
	}
	return client
}

func NewDefault(gitRemote ...GitRemote) (*Client, error) {
	options := api.ClientOptions{
		Host:         "github.com",
		LogIgnoreEnv: true,
		Headers:      map[string]string{"Cache-Control": "no-cache"},
	}
	rest, err := api.NewRESTClient(options)
	if err != nil {
		return nil, fmt.Errorf("create GitHub API client: %w", err)
	}
	graphql, err := api.NewGraphQLClient(options)
	if err != nil {
		return nil, fmt.Errorf("create GitHub GraphQL client: %w", err)
	}
	client := New(rest)
	client.graphql = graphql
	if len(gitRemote) > 0 {
		client.pushProbe = gitRemote[0]
		client.refResolver = gitRemote[0]
	}
	return client, nil
}

func (c *Client) ReadSkill(
	ctx context.Context,
	repository status.Repository,
	skillPath string,
	revision string,
) (source.SkillSnapshot, error) {
	cleanPath, err := validateSkillPath(skillPath)
	if err != nil {
		return source.SkillSnapshot{}, err
	}
	if revision == "" {
		return source.SkillSnapshot{}, fmt.Errorf("revision is required")
	}
	resolvedRevision, err := c.resolveRevision(ctx, repository, revision)
	if err != nil {
		return source.SkillSnapshot{}, err
	}
	tree, err := c.readTree(ctx, repository, resolvedRevision)
	if err != nil {
		return source.SkillSnapshot{}, err
	}

	prefix := ""
	if cleanPath != "" {
		prefix = cleanPath + "/"
	}
	files := make(map[string][]byte)
	executable := make(map[string]bool)
	skillTreeSHA := tree.SHA
	foundSkillPath := cleanPath == ""
	for _, entry := range tree.Entries {
		relative := entry.Path
		if cleanPath != "" {
			if entry.Path == cleanPath && entry.Type == "tree" {
				foundSkillPath = true
				skillTreeSHA = entry.SHA
				continue
			}
			if !strings.HasPrefix(entry.Path, prefix) {
				continue
			}
			foundSkillPath = true
			relative = strings.TrimPrefix(entry.Path, prefix)
		}
		if entry.Type == "tree" {
			continue
		}
		if entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755") {
			return source.SkillSnapshot{}, fmt.Errorf("unsupported remote entry %s with mode %s and type %s", entry.Path, entry.Mode, entry.Type)
		}
		content, err := c.readBlob(ctx, repository, entry.SHA)
		if err != nil {
			return source.SkillSnapshot{}, fmt.Errorf("read %s: %w", entry.Path, err)
		}
		files[relative] = append([]byte(nil), content...)
		executable[relative] = entry.Mode == "100755"
		if relative == "SKILL.md" {
			if _, err := skill.ParseName(content); err != nil {
				return source.SkillSnapshot{}, fmt.Errorf("parse remote SKILL.md: %w", err)
			}
		}
	}
	if !foundSkillPath {
		return source.SkillSnapshot{}, fmt.Errorf("skill path %q does not exist at %s", cleanPath, revision)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return source.SkillSnapshot{}, fmt.Errorf("skill path %q has no SKILL.md at %s", cleanPath, revision)
	}
	return source.SkillSnapshot{CommitSHA: resolvedRevision, TreeSHA: skillTreeSHA, Files: files, Executable: executable}, nil
}

func (c *Client) ReadRepositoryTree(
	ctx context.Context,
	repository status.Repository,
	revision string,
) (source.RepositoryTree, error) {
	if revision == "" {
		return source.RepositoryTree{}, fmt.Errorf("revision is required")
	}
	resolvedRevision, err := c.resolveRevision(ctx, repository, revision)
	if err != nil {
		return source.RepositoryTree{}, err
	}
	tree, err := c.readTree(ctx, repository, resolvedRevision)
	if err != nil {
		return source.RepositoryTree{}, err
	}
	entries := make([]source.TreeEntry, len(tree.Entries))
	for index, entry := range tree.Entries {
		entries[index] = source.TreeEntry{
			Path: entry.Path,
			Mode: entry.Mode,
			Type: entry.Type,
			SHA:  entry.SHA,
		}
	}
	return source.RepositoryTree{SHA: tree.SHA, Entries: entries}, nil
}

func (c *Client) DiscoverSkills(
	ctx context.Context,
	repository status.Repository,
	revision string,
) (discovery.Result, error) {
	if revision == "" {
		return discovery.Result{}, fmt.Errorf("revision is required")
	}
	resolvedRevision, err := c.resolveRevision(ctx, repository, revision)
	if err != nil {
		return discovery.Result{}, err
	}
	tree, err := c.readTree(ctx, repository, resolvedRevision)
	if err != nil {
		return discovery.Result{}, err
	}
	entries := make([]discovery.Entry, len(tree.Entries))
	for index, entry := range tree.Entries {
		entries[index] = discovery.Entry{Path: entry.Path, Type: entry.Type, SHA: entry.SHA}
	}
	return discovery.FromTree(resolvedRevision, entries)
}

func (c *Client) resolveRevision(ctx context.Context, repository status.Repository, revision string) (string, error) {
	if !strings.HasPrefix(revision, "refs/") {
		return revision, nil
	}
	if c.refResolver != nil {
		repositoryURL := fmt.Sprintf("https://github.com/%s/%s.git", repository.Owner, repository.Name)
		return c.refResolver.ResolveRef(ctx, repositoryURL, revision)
	}
	return c.resolveRef(ctx, repository, strings.TrimPrefix(revision, "refs/"))
}

func (c *Client) ResolveSourceRef(
	ctx context.Context,
	repository source.Repository,
	fullRef string,
) (source.ResolvedRef, error) {
	if _, err := source.ParseRef(fullRef); err != nil {
		return source.ResolvedRef{}, err
	}
	refObject, err := c.readRefObject(ctx, repository, strings.TrimPrefix(fullRef, "refs/"))
	if err != nil {
		return source.ResolvedRef{}, err
	}
	resolved := source.ResolvedRef{RefSHA: refObject.SHA}
	object := refObject
	for depth := 0; depth < annotatedTagPeelLimit; depth++ {
		switch object.Type {
		case "commit":
			resolved.CommitSHA = object.SHA
			return resolved, nil
		case "tag":
			object, err = c.readTagTarget(ctx, repository, object.SHA)
			if err != nil {
				return source.ResolvedRef{}, err
			}
		default:
			return source.ResolvedRef{}, fmt.Errorf("source ref %s does not resolve to a commit", fullRef)
		}
	}
	return source.ResolvedRef{}, fmt.Errorf("source ref %s exceeds annotated tag peel limit", fullRef)
}

func (c *Client) readTree(ctx context.Context, repository status.Repository, revision string) (treeResponse, error) {
	key := repositoryRevision{owner: repository.Owner, name: repository.Name, revision: revision}
	c.treeMu.Lock()
	if tree, ok := c.trees[key]; ok {
		c.treeMu.Unlock()
		return tree, nil
	}
	c.treeMu.Unlock()

	endpoint := fmt.Sprintf(
		"repos/%s/%s/git/trees/%s?recursive=1",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		url.PathEscape(revision),
	)
	var tree treeResponse
	if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &tree); err != nil {
		return treeResponse{}, fmt.Errorf("read GitHub tree %s: %w", revision, err)
	}
	if tree.Truncated {
		return treeResponse{}, fmt.Errorf("GitHub tree %s is truncated", revision)
	}
	c.treeMu.Lock()
	if c.trees == nil {
		c.trees = make(map[repositoryRevision]treeResponse)
	}
	c.trees[key] = tree
	c.treeMu.Unlock()
	return tree, nil
}

func (c *Client) resolveRef(ctx context.Context, repository status.Repository, ref string) (string, error) {
	resolved, err := c.ResolveSourceRef(ctx, repository, "refs/"+ref)
	if err != nil {
		return "", err
	}
	return resolved.CommitSHA, nil
}

func (c *Client) readRefObject(ctx context.Context, repository source.Repository, ref string) (gitObject, error) {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/git/ref/%s",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		url.PathEscape(ref),
	)
	var response struct {
		Object gitObject `json:"object"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return gitObject{}, fmt.Errorf("resolve source ref %s: %w", ref, err)
	}
	if response.Object.SHA == "" || response.Object.Type == "" {
		return gitObject{}, fmt.Errorf("resolve source ref %s: incomplete object", ref)
	}
	return response.Object, nil
}

func (c *Client) readTagTarget(ctx context.Context, repository source.Repository, sha string) (gitObject, error) {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/git/tags/%s",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		url.PathEscape(sha),
	)
	var response struct {
		Object gitObject `json:"object"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return gitObject{}, fmt.Errorf("peel annotated tag %s: %w", sha, err)
	}
	if response.Object.SHA == "" || response.Object.Type == "" {
		return gitObject{}, fmt.Errorf("peel annotated tag %s: incomplete target", sha)
	}
	return response.Object, nil
}

func (c *Client) ReadPermission(ctx context.Context, repository status.Repository, branch string) (bool, error) {
	canPush, err := c.ReadRepositoryPermission(ctx, repository)
	if err != nil || canPush || c.pushProbe == nil {
		return canPush, err
	}
	repositoryURL := fmt.Sprintf("https://github.com/%s/%s.git", repository.Owner, repository.Name)
	return c.pushProbe.CanPush(ctx, repositoryURL, branch)
}

func (c *Client) ReadRepositoryPermission(ctx context.Context, repository status.Repository) (bool, error) {
	endpoint := fmt.Sprintf("repos/%s/%s", url.PathEscape(repository.Owner), url.PathEscape(repository.Name))
	var response struct {
		Permissions struct {
			Push *bool `json:"push"`
		} `json:"permissions"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return false, fmt.Errorf("read repository permission: %w", err)
	}
	if response.Permissions.Push == nil {
		return false, fmt.Errorf("read repository permission: response is missing permissions.push")
	}
	return *response.Permissions.Push, nil
}

func (c *Client) readBlob(ctx context.Context, repository status.Repository, sha string) ([]byte, error) {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/git/blobs/%s",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		url.PathEscape(sha),
	)
	var response struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	if response.Encoding != "base64" {
		return nil, fmt.Errorf("unsupported blob encoding %q", response.Encoding)
	}
	content, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return nil, fmt.Errorf("decode blob: %w", err)
	}
	return content, nil
}

func validateSkillPath(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("unsafe skill path %q", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return "", fmt.Errorf("unsafe skill path %q", value)
	}
	return clean, nil
}
