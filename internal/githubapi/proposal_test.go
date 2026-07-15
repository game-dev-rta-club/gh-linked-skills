package githubapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

func TestListPullRequestsMapsAndPaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet || request.URL.Path != "/repos/owner/repo/pulls" {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("state") != "open" || request.URL.Query().Get("base") != "main" ||
			request.URL.Query().Get("per_page") != "100" {
			http.Error(writer, "unexpected query", http.StatusBadRequest)
			return
		}
		page, _ := strconv.Atoi(request.URL.Query().Get("page"))
		count := 100
		if page == 2 {
			count = 1
		}
		items := make([]map[string]any, count)
		for index := range items {
			number := (page-1)*100 + index + 1
			items[index] = map[string]any{
				"number": number, "html_url": fmt.Sprintf("https://github.com/owner/repo/pull/%d", number),
				"state": "open", "body": "body", "merged_at": nil,
				"head": map[string]any{"ref": "skill-linker/sample", "sha": "head-sha", "repo": map[string]any{"full_name": "owner/repo"}},
				"base": map[string]any{"ref": "main", "repo": map[string]any{"full_name": "owner/repo"}},
			}
		}
		_ = json.NewEncoder(writer).Encode(items)
	}))
	defer server.Close()

	result, err := newTestClient(t, server).ListPullRequests(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		proposal.ListOptions{State: "open", Base: "main"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(result) != 101 {
		t.Fatalf("requests=%d pulls=%d", requests, len(result))
	}
	first := result[0]
	if first.Number != 1 || first.HeadRef != "skill-linker/sample" || first.HeadRepository != "owner/repo" ||
		first.BaseRef != "main" || first.Merged {
		t.Fatalf("first pull = %#v", first)
	}
}

func TestCreatePullRequestPostsExplicitHeadAndBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/repos/owner/repo/pulls" {
			http.NotFound(writer, request)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["title"] != "Sync sample" || body["head"] != "proposal-branch" || body["base"] != "main" ||
			body["body"] != "proposal body" || body["draft"] != false {
			t.Fatalf("request body = %#v", body)
		}
		writer.WriteHeader(http.StatusCreated)
		fmt.Fprint(writer, `{"number":42,"html_url":"https://github.com/owner/repo/pull/42","state":"open","body":"proposal body","head":{"ref":"proposal-branch","sha":"head-sha","repo":{"full_name":"owner/repo"}},"base":{"ref":"main","repo":{"full_name":"owner/repo"}}}`)
	}))
	defer server.Close()

	created, err := newTestClient(t, server).CreatePullRequest(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"},
		proposal.CreateRequest{Title: "Sync sample", Head: "proposal-branch", Base: "main", Body: "proposal body"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Number != 42 || created.URL != "https://github.com/owner/repo/pull/42" {
		t.Fatalf("created = %#v", created)
	}
}

func TestUpdatePullRequestBodyReturnsCurrentHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPatch || request.URL.Path != "/repos/owner/repo/pulls/42" {
			http.NotFound(writer, request)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["body"] != "updated" {
			t.Fatalf("request body = %#v", body)
		}
		fmt.Fprint(writer, `{"number":42,"html_url":"https://github.com/owner/repo/pull/42","state":"open","body":"updated","head":{"ref":"proposal-branch","sha":"new-head","repo":{"full_name":"owner/repo"}},"base":{"ref":"main","repo":{"full_name":"owner/repo"}}}`)
	}))
	defer server.Close()

	updated, err := newTestClient(t, server).UpdatePullRequestBody(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"}, 42, "updated",
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != "updated" || updated.HeadSHA != "new-head" {
		t.Fatalf("updated = %#v", updated)
	}
}
