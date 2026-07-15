package githubapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

const pullRequestPageSize = 100

type pullRequestResponse struct {
	Number   int     `json:"number"`
	HTMLURL  string  `json:"html_url"`
	State    string  `json:"state"`
	Body     string  `json:"body"`
	MergedAt *string `json:"merged_at"`
	Head     struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
}

func (c *Client) ListPullRequests(
	ctx context.Context,
	repository source.Repository,
	options proposal.ListOptions,
) ([]proposal.PullRequest, error) {
	result := make([]proposal.PullRequest, 0)
	for page := 1; ; page++ {
		query := url.Values{}
		if options.State != "" {
			query.Set("state", options.State)
		}
		if options.Base != "" {
			query.Set("base", options.Base)
		}
		if options.Head != "" {
			query.Set("head", options.Head)
		}
		query.Set("per_page", strconv.Itoa(pullRequestPageSize))
		query.Set("page", strconv.Itoa(page))
		endpoint := fmt.Sprintf(
			"repos/%s/%s/pulls?%s",
			url.PathEscape(repository.Owner), url.PathEscape(repository.Name), query.Encode(),
		)
		var response []pullRequestResponse
		if err := c.rest.DoWithContext(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return nil, fmt.Errorf("list pull requests for %s/%s: %w", repository.Owner, repository.Name, err)
		}
		for _, item := range response {
			mapped, err := mapPullRequest(item)
			if err != nil {
				return nil, err
			}
			result = append(result, mapped)
		}
		if len(response) < pullRequestPageSize {
			return result, nil
		}
	}
}

func (c *Client) CreatePullRequest(
	ctx context.Context,
	repository source.Repository,
	request proposal.CreateRequest,
) (proposal.PullRequest, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls", url.PathEscape(repository.Owner), url.PathEscape(repository.Name))
	body := map[string]any{
		"title": request.Title,
		"head":  request.Head,
		"base":  request.Base,
		"body":  request.Body,
		"draft": false,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return proposal.PullRequest{}, fmt.Errorf("encode pull request: %w", err)
	}
	var response pullRequestResponse
	if err := c.rest.DoWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded), &response); err != nil {
		return proposal.PullRequest{}, fmt.Errorf("create pull request for %s/%s: %w", repository.Owner, repository.Name, err)
	}
	return mapPullRequest(response)
}

func (c *Client) UpdatePullRequestBody(
	ctx context.Context,
	repository source.Repository,
	number int,
	body string,
) (proposal.PullRequest, error) {
	endpoint := fmt.Sprintf(
		"repos/%s/%s/pulls/%d", url.PathEscape(repository.Owner), url.PathEscape(repository.Name), number,
	)
	encoded, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return proposal.PullRequest{}, fmt.Errorf("encode pull request update: %w", err)
	}
	var response pullRequestResponse
	if err := c.rest.DoWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(encoded), &response); err != nil {
		return proposal.PullRequest{}, fmt.Errorf("update pull request %d: %w", number, err)
	}
	return mapPullRequest(response)
}

func mapPullRequest(response pullRequestResponse) (proposal.PullRequest, error) {
	if response.Number <= 0 || response.HTMLURL == "" || response.State == "" || response.Head.Ref == "" ||
		response.Head.SHA == "" || response.Head.Repo.FullName == "" || response.Base.Ref == "" ||
		response.Base.Repo.FullName == "" {
		return proposal.PullRequest{}, fmt.Errorf("GitHub returned an incomplete pull request")
	}
	return proposal.PullRequest{
		Number: response.Number, URL: response.HTMLURL, State: response.State, Body: response.Body,
		Merged: response.MergedAt != nil, HeadRef: response.Head.Ref, HeadSHA: response.Head.SHA,
		HeadRepository: response.Head.Repo.FullName, BaseRef: response.Base.Ref,
		BaseRepository: response.Base.Repo.FullName,
	}, nil
}
