package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/source"
	"github.com/game-dev-rta-club/gh-linked-skills/internal/status"
)

type graphQLCall struct {
	query     string
	variables map[string]interface{}
}

type scriptedGraphQL struct {
	responses []string
	errors    []error
	calls     []graphQLCall
}

func (f *scriptedGraphQL) DoWithContext(
	_ context.Context,
	query string,
	variables map[string]interface{},
	response interface{},
) error {
	index := len(f.calls)
	copyVariables := make(map[string]interface{}, len(variables))
	for key, value := range variables {
		copyVariables[key] = value
	}
	f.calls = append(f.calls, graphQLCall{query: query, variables: copyVariables})
	if index < len(f.responses) && f.responses[index] != "" {
		if err := json.Unmarshal([]byte(f.responses[index]), response); err != nil {
			return err
		}
	}
	if index < len(f.errors) {
		return f.errors[index]
	}
	return nil
}

func TestReadStatusPreflightBatchesRefsAndPermissions(t *testing.T) {
	graphql := &scriptedGraphQL{responses: []string{`{
		"repo0": {
			"viewerPermission": "READ",
			"ref0": {"target": {"__typename": "Tag", "oid": "tag-object", "target": {"__typename": "Commit", "oid": "tag-commit"}}}
		},
		"repo1": {
			"viewerPermission": "ADMIN",
			"ref0": {"target": {"__typename": "Commit", "oid": "main-commit"}}
		}
	}`}}
	client := &Client{graphql: graphql}
	requests := []status.PreflightRequest{
		{Repository: source.Repository{Owner: "nikollson", Name: "repo"}, Refs: []string{"refs/heads/main", "refs/heads/main"}, ReadPermission: true},
		{Repository: source.Repository{Owner: "addyosmani", Name: "skills"}, Refs: []string{"refs/tags/v1"}, ReadPermission: true},
	}

	results := client.ReadStatusPreflight(context.Background(), requests)

	if len(graphql.calls) != 1 {
		t.Fatalf("GraphQL calls = %d, want 1", len(graphql.calls))
	}
	call := graphql.calls[0]
	for _, raw := range []string{"nikollson", "addyosmani", "refs/heads/main", "refs/tags/v1"} {
		if strings.Contains(call.query, raw) {
			t.Fatalf("query contains raw input %q: %s", raw, call.query)
		}
	}
	addy := results["addyosmani/skills"]
	if !addy.PermissionChecked || addy.CanPush || addy.PermissionErr != nil {
		t.Fatalf("addy permission = %#v", addy)
	}
	if got := addy.Refs["refs/tags/v1"]; got.Err != nil || got.Resolved.RefSHA != "tag-object" || got.Resolved.CommitSHA != "tag-commit" {
		t.Fatalf("tag result = %#v", got)
	}
	owned := results["nikollson/repo"]
	if !owned.CanPush || owned.PermissionErr != nil {
		t.Fatalf("owned permission = %#v", owned)
	}
	if got := owned.Refs["refs/heads/main"]; got.Err != nil || got.Resolved.CommitSHA != "main-commit" {
		t.Fatalf("branch result = %#v", got)
	}
}

func TestReadStatusPreflightKeepsPartialGraphQLFailuresLocal(t *testing.T) {
	graphql := &scriptedGraphQL{
		responses: []string{`{
			"repo0": {
				"viewerPermission": "ADMIN",
				"ref0": null,
				"ref1": {"target": {"__typename": "Commit", "oid": "release-commit"}}
			}
		}`},
		errors: []error{&api.GraphQLError{Errors: []api.GraphQLErrorItem{{
			Message: "ref unavailable", Path: []interface{}{"repo0", "ref0"},
		}}}},
	}
	client := &Client{graphql: graphql}

	results := client.ReadStatusPreflight(context.Background(), []status.PreflightRequest{{
		Repository: source.Repository{Owner: "owner", Name: "repo"},
		Refs:       []string{"refs/heads/main", "refs/heads/release"}, ReadPermission: true,
	}})

	result := results["owner/repo"]
	if result.PermissionErr != nil || !result.CanPush {
		t.Fatalf("permission = %#v", result)
	}
	if result.Refs["refs/heads/main"].Err == nil {
		t.Fatalf("main result = %#v", result.Refs["refs/heads/main"])
	}
	if got := result.Refs["refs/heads/release"]; got.Err != nil || got.Resolved.CommitSHA != "release-commit" {
		t.Fatalf("release result = %#v", got)
	}
}

func TestReadStatusPreflightTreatsNullPermissionAsUnknown(t *testing.T) {
	graphql := &scriptedGraphQL{responses: []string{`{
		"repo0": {
			"viewerPermission": null,
			"ref0": {"target": {"__typename": "Commit", "oid": "commit"}}
		}
	}`}}
	client := &Client{graphql: graphql}

	results := client.ReadStatusPreflight(context.Background(), []status.PreflightRequest{{
		Repository: source.Repository{Owner: "owner", Name: "repo"},
		Refs:       []string{"refs/heads/main"}, ReadPermission: true,
	}})

	result := results["owner/repo"]
	if !result.PermissionChecked || result.PermissionErr == nil {
		t.Fatalf("permission = %#v", result)
	}
	if result.Refs["refs/heads/main"].Err != nil {
		t.Fatalf("ref = %#v", result.Refs["refs/heads/main"])
	}
}

func TestReadStatusPreflightTreatsTransportFailureAsChunkWide(t *testing.T) {
	graphql := &scriptedGraphQL{errors: []error{errors.New("network unavailable")}}
	client := &Client{graphql: graphql}

	results := client.ReadStatusPreflight(context.Background(), []status.PreflightRequest{{
		Repository: source.Repository{Owner: "owner", Name: "repo"},
		Refs:       []string{"refs/heads/main", "refs/heads/release"}, ReadPermission: true,
	}})

	result := results["owner/repo"]
	if !result.PermissionChecked || result.PermissionErr == nil {
		t.Fatalf("permission = %#v", result)
	}
	for _, ref := range []string{"refs/heads/main", "refs/heads/release"} {
		if result.Refs[ref].Err == nil {
			t.Fatalf("ref %s = %#v", ref, result.Refs[ref])
		}
	}
}

func TestReadStatusPreflightDoesNotFallBackToSerialREST(t *testing.T) {
	restCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		restCalls++
		http.Error(writer, "unexpected REST request", http.StatusInternalServerError)
	}))
	defer server.Close()
	client := newTestClient(t, server)

	results := client.ReadStatusPreflight(context.Background(), []status.PreflightRequest{{
		Repository: source.Repository{Owner: "owner", Name: "repo"},
		Refs:       []string{"refs/heads/main"}, ReadPermission: true,
	}})

	result := results["owner/repo"]
	if restCalls != 0 {
		t.Fatalf("REST calls = %d, want 0", restCalls)
	}
	if !result.PermissionChecked || result.PermissionErr == nil || result.Refs["refs/heads/main"].Err == nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestReadStatusPreflightChunksLargeRefSets(t *testing.T) {
	refs := make([]string, statusPreflightChunkRefs+1)
	for index := range refs {
		refs[index] = fmt.Sprintf("refs/heads/branch-%02d", index)
	}
	sort.Strings(refs)
	graphql := &scriptedGraphQL{responses: []string{
		preflightResponse(0, refs[:statusPreflightChunkRefs]),
		preflightResponse(statusPreflightChunkRefs, refs[statusPreflightChunkRefs:]),
	}}
	client := &Client{graphql: graphql}

	results := client.ReadStatusPreflight(context.Background(), []status.PreflightRequest{{
		Repository: source.Repository{Owner: "owner", Name: "repo"}, Refs: refs, ReadPermission: true,
	}})

	if len(graphql.calls) != 2 {
		t.Fatalf("GraphQL calls = %d, want 2", len(graphql.calls))
	}
	result := results["owner/repo"]
	if len(result.Refs) != len(refs) || !result.PermissionChecked || result.PermissionErr != nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestResolveGraphQLTargetMatchesAnnotatedTagDepthLimit(t *testing.T) {
	withinLimit := nestedTagTarget(15)
	resolved, err := resolveGraphQLTarget("refs/tags/v1", withinLimit)
	if err != nil || resolved.CommitSHA != "commit" {
		t.Fatalf("within limit = %#v, %v", resolved, err)
	}
	if _, err := resolveGraphQLTarget("refs/tags/v1", nestedTagTarget(16)); err == nil || !strings.Contains(err.Error(), "peel limit") {
		t.Fatalf("over limit error = %v", err)
	}
}

func preflightResponse(offset int, refs []string) string {
	fields := []string{`"viewerPermission":"ADMIN"`}
	for index := range refs {
		fields = append(fields, fmt.Sprintf(`"ref%d":{"target":{"__typename":"Commit","oid":"commit-%d"}}`, index, offset+index))
	}
	return `{"repo0":{` + strings.Join(fields, ",") + `}}`
}

func nestedTagTarget(tags int) graphQLTarget {
	target := graphQLTarget{TypeName: "Commit", OID: "commit"}
	for index := 0; index < tags; index++ {
		copy := target
		target = graphQLTarget{TypeName: "Tag", OID: fmt.Sprintf("tag-%d", tags-index), Target: &copy}
	}
	return target
}

func TestGoGHGraphQLPopulatesPartialDataBeforeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, `{"data":{"value":"decoded"},"errors":[{"message":"partial","path":["value"]}]}`)
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	graphql, err := api.NewGraphQLClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: rewriteTransport{target: target, base: http.DefaultTransport},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Value string `json:"value"`
	}

	err = graphql.DoWithContext(context.Background(), "query { value }", nil, &response)

	var graphQLErr *api.GraphQLError
	if !errors.As(err, &graphQLErr) || response.Value != "decoded" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
}
