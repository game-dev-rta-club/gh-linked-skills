package githubapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/discovery"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/status"
)

type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func TestReadSkillFromRepositoryTree(t *testing.T) {
	document := "---\nname: sample\ndescription: Example skill.\n---\nBody\n"
	script := "export default true;\n"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/git/ref/"):
			fmt.Fprint(writer, `{"object":{"type":"commit","sha":"resolved-commit"}}`)
		case strings.Contains(request.URL.Path, "/git/trees/"):
			if !strings.HasSuffix(request.URL.Path, "/resolved-commit") {
				http.Error(writer, "tree request did not use resolved commit", http.StatusBadRequest)
				return
			}
			fmt.Fprint(writer, `{"sha":"root","truncated":false,"tree":[`+
				`{"path":"skills/sample","mode":"040000","type":"tree","sha":"skill-tree"},`+
				`{"path":"skills/sample/SKILL.md","mode":"100644","type":"blob","sha":"doc"},`+
				`{"path":"skills/sample/scripts/check.mjs","mode":"100755","type":"blob","sha":"script"}`+
				`]}`)
		case strings.HasSuffix(request.URL.Path, "/git/blobs/doc"):
			fmt.Fprintf(writer, `{"encoding":"base64","content":%q}`, base64.StdEncoding.EncodeToString([]byte(document)))
		case strings.HasSuffix(request.URL.Path, "/git/blobs/script"):
			fmt.Fprintf(writer, `{"encoding":"base64","content":%q}`, base64.StdEncoding.EncodeToString([]byte(script)))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	snapshot, err := client.ReadSkill(
		context.Background(),
		status.Repository{Owner: "owner", Name: "repo"},
		"skills/sample",
		"refs/heads/main",
	)

	if err != nil {
		t.Fatalf("ReadSkill() error = %v", err)
	}
	if _, ok := snapshot.Files["SKILL.md"]; !ok {
		t.Fatal("snapshot missing canonical SKILL.md")
	}
	if string(snapshot.Files["scripts/check.mjs"]) != script || !snapshot.Executable["scripts/check.mjs"] {
		t.Fatalf("script = %q mode=%v, want executable", snapshot.Files["scripts/check.mjs"], snapshot.Executable)
	}
}

func TestReadSkillAcceptsSkillTreeSHA(t *testing.T) {
	document := "---\nname: sample\ndescription: Example skill.\n---\nBody\n"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/git/trees/"):
			fmt.Fprint(writer, `{"sha":"skill-tree","truncated":false,"tree":[{"path":"SKILL.md","mode":"100644","type":"blob","sha":"doc"}]}`)
		case strings.HasSuffix(request.URL.Path, "/git/blobs/doc"):
			fmt.Fprintf(writer, `{"encoding":"base64","content":%q}`, base64.StdEncoding.EncodeToString([]byte(document)))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	snapshot, err := newTestClient(t, server).ReadSkill(
		context.Background(),
		status.Repository{Owner: "owner", Name: "repo"},
		"",
		"skill-tree",
	)

	if err != nil {
		t.Fatalf("ReadSkill() error = %v", err)
	}
	if len(snapshot.Files) != 1 || snapshot.Files["SKILL.md"] == nil {
		t.Fatalf("snapshot = %#v, want one SKILL.md", snapshot)
	}
}

func TestReadRepositoryTreeReturnsMetadataWithoutReadingBlobs(t *testing.T) {
	treeCalls := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/git/trees/") {
			treeCalls.Add(1)
			fmt.Fprint(writer, `{"sha":"root","truncated":false,"tree":[`+
				`{"path":"skills/sample","mode":"040000","type":"tree","sha":"skill-tree"},`+
				`{"path":"skills/sample/SKILL.md","mode":"100644","type":"blob","sha":"doc"}`+
				`]}`)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	tree, err := newTestClient(t, server).ReadRepositoryTree(
		context.Background(),
		status.Repository{Owner: "owner", Name: "repo"},
		"commit-sha",
	)

	if err != nil {
		t.Fatalf("ReadRepositoryTree() error = %v", err)
	}
	if tree.SHA != "root" || len(tree.Entries) != 2 || tree.Entries[0].Path != "skills/sample" {
		t.Fatalf("tree = %#v", tree)
	}
	if treeCalls.Load() != 1 {
		t.Fatalf("tree calls = %d, want 1", treeCalls.Load())
	}
}

func TestRepositoryTreeAndSkillSnapshotShareTreeRequest(t *testing.T) {
	treeCalls := atomic.Int32{}
	document := []byte("---\nname: sample\ndescription: Example skill.\n---\nBody\n")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/git/trees/"):
			treeCalls.Add(1)
			fmt.Fprint(writer, `{"sha":"root","truncated":false,"tree":[`+
				`{"path":"skills/sample","mode":"040000","type":"tree","sha":"skill-tree"},`+
				`{"path":"skills/sample/SKILL.md","mode":"100644","type":"blob","sha":"doc"}`+
				`]}`)
		case strings.HasSuffix(request.URL.Path, "/git/blobs/doc"):
			fmt.Fprintf(writer, `{"encoding":"base64","content":%q}`, base64.StdEncoding.EncodeToString(document))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server)

	if _, err := client.ReadRepositoryTree(context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadSkill(context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "skills/sample", "commit"); err != nil {
		t.Fatal(err)
	}
	if treeCalls.Load() != 1 {
		t.Fatalf("tree calls = %d, want 1", treeCalls.Load())
	}
}

func TestReadSkillRejectsTruncatedTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, `{"sha":"root","truncated":true,"tree":[]}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ReadSkill(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "skills/sample", "main",
	)

	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("ReadSkill() error = %v, want truncated tree error", err)
	}
}

func TestReadSkillRejectsTraversalBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ReadSkill(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "../secret", "main",
	)

	if err == nil || !strings.Contains(err.Error(), "unsafe skill path") {
		t.Fatalf("ReadSkill() error = %v, want unsafe path", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestDiscoverSkillsUsesOneResolvedRepositoryTree(t *testing.T) {
	var refRequests atomic.Int32
	var treeRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/git/ref/"):
			refRequests.Add(1)
			fmt.Fprint(writer, `{"object":{"type":"commit","sha":"resolved-commit"}}`)
		case strings.Contains(request.URL.Path, "/git/trees/resolved-commit"):
			treeRequests.Add(1)
			fmt.Fprint(writer, `{"sha":"resolved-commit","truncated":false,"tree":[`+
				`{"path":"skills/alpha","mode":"040000","type":"tree","sha":"tree-alpha"},`+
				`{"path":"skills/alpha/SKILL.md","mode":"100644","type":"blob","sha":"doc-alpha"},`+
				`{"path":"beta","mode":"040000","type":"tree","sha":"tree-beta"},`+
				`{"path":"beta/SKILL.md","mode":"100644","type":"blob","sha":"doc-beta"}`+
				`]}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	result, err := newTestClient(t, server).DiscoverSkills(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "refs/heads/main",
	)

	if err != nil {
		t.Fatalf("DiscoverSkills() error = %v", err)
	}
	if result.CommitSHA != "resolved-commit" || len(result.Skills) != 2 {
		t.Fatalf("DiscoverSkills() = %#v", result)
	}
	if result.Skills[0] != (discovery.Skill{Name: "alpha", Path: "skills/alpha", TreeSHA: "tree-alpha"}) {
		t.Fatalf("first skill = %#v", result.Skills[0])
	}
	if refRequests.Load() != 1 || treeRequests.Load() != 1 {
		t.Fatalf("ref requests = %d, tree requests = %d", refRequests.Load(), treeRequests.Load())
	}
}

func TestDiscoverSkillsRejectsTruncatedRepositoryTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/git/trees/") {
			fmt.Fprint(writer, `{"sha":"commit","truncated":true,"tree":[]}`)
			return
		}
		fmt.Fprint(writer, `{"object":{"type":"commit","sha":"commit"}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).DiscoverSkills(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "refs/heads/main",
	)
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("DiscoverSkills() error = %v, want truncated", err)
	}
}

func TestResolveSourceRefSupportsLightweightTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.Contains(request.URL.Path, "/git/ref/tags/v1.2.0") {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"object":{"type":"commit","sha":"commit-sha"}}`)
	}))
	defer server.Close()

	resolved, err := newTestClient(t, server).ResolveSourceRef(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"}, "refs/tags/v1.2.0",
	)

	if err != nil || resolved.RefSHA != "commit-sha" || resolved.CommitSHA != "commit-sha" {
		t.Fatalf("ResolveSourceRef() = %#v, %v", resolved, err)
	}
}

func TestResolveSourceRefPeelsAnnotatedTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/git/ref/tags/v1.2.0"):
			fmt.Fprint(writer, `{"object":{"type":"tag","sha":"tag-object"}}`)
		case strings.HasSuffix(request.URL.Path, "/git/tags/tag-object"):
			fmt.Fprint(writer, `{"object":{"type":"commit","sha":"commit-sha"}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	resolved, err := newTestClient(t, server).ResolveSourceRef(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"}, "refs/tags/v1.2.0",
	)

	if err != nil || resolved.RefSHA != "tag-object" || resolved.CommitSHA != "commit-sha" {
		t.Fatalf("ResolveSourceRef() = %#v, %v", resolved, err)
	}
}

func TestResolveSourceRefSupportsSlashTagAndNestedAnnotatedTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.EscapedPath(), "/git/ref/tags%2Freleases%2Fv1.2.0"):
			fmt.Fprint(writer, `{"object":{"type":"tag","sha":"outer-tag"}}`)
		case strings.HasSuffix(request.URL.Path, "/git/tags/outer-tag"):
			fmt.Fprint(writer, `{"object":{"type":"tag","sha":"inner-tag"}}`)
		case strings.HasSuffix(request.URL.Path, "/git/tags/inner-tag"):
			fmt.Fprint(writer, `{"object":{"type":"commit","sha":"commit-sha"}}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	resolved, err := newTestClient(t, server).ResolveSourceRef(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"}, "refs/tags/releases/v1.2.0",
	)

	if err != nil || resolved.RefSHA != "outer-tag" || resolved.CommitSHA != "commit-sha" {
		t.Fatalf("ResolveSourceRef() = %#v, %v", resolved, err)
	}
}

func TestResolveSourceRefRejectsNonCommitTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, `{"object":{"type":"tree","sha":"tree-sha"}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ResolveSourceRef(
		context.Background(), source.Repository{Owner: "owner", Name: "repo"}, "refs/tags/v1.2.0",
	)

	if err == nil || !strings.Contains(err.Error(), "does not resolve to a commit") {
		t.Fatalf("ResolveSourceRef() error = %v", err)
	}
}

func TestReadPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repo" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"permissions":{"push":true}}`)
	}))
	defer server.Close()

	canPush, err := newTestClient(t, server).ReadPermission(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "main",
	)

	if err != nil {
		t.Fatalf("ReadPermission() error = %v", err)
	}
	if !canPush {
		t.Fatal("ReadPermission() = false, want true")
	}
}

type fakePushProbe struct {
	canPush bool
	calls   int
}

func (f *fakePushProbe) CanPush(context.Context, string, string) (bool, error) {
	f.calls++
	return f.canPush, nil
}

func TestReadPermissionFallsBackToDryRunProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repo" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"permissions":{"push":false}}`)
	}))
	defer server.Close()
	probe := &fakePushProbe{canPush: true}
	client := newTestClientWithProbe(t, server, probe)

	canPush, err := client.ReadPermission(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"}, "test/e2e/run",
	)

	if err != nil {
		t.Fatalf("ReadPermission() error = %v", err)
	}
	if !canPush || probe.calls != 1 {
		t.Fatalf("canPush = %v, probe calls = %d", canPush, probe.calls)
	}
}

func TestReadRepositoryPermissionDoesNotProbeGit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repo" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, `{"permissions":{"push":false}}`)
	}))
	defer server.Close()
	probe := &fakePushProbe{canPush: true}
	client := newTestClientWithProbe(t, server, probe)

	canPush, err := client.ReadRepositoryPermission(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"},
	)

	if err != nil {
		t.Fatalf("ReadRepositoryPermission() error = %v", err)
	}
	if canPush || probe.calls != 0 {
		t.Fatalf("canPush = %v, probe calls = %d", canPush, probe.calls)
	}
}

func TestReadRepositoryPermissionRejectsMissingPushField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, `{"permissions":{}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ReadRepositoryPermission(
		context.Background(), status.Repository{Owner: "owner", Name: "repo"},
	)

	if err == nil || !strings.Contains(err.Error(), "permissions.push") {
		t.Fatalf("ReadRepositoryPermission() error = %v", err)
	}
}

func TestRepositoryTreeCacheIsScopedByRepositoryAndRevision(t *testing.T) {
	requests := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		parts := strings.Split(request.URL.Path, "/")
		if len(parts) < 7 {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, `{"sha":%q,"truncated":false,"tree":[]}`, parts[3]+"-"+parts[6])
	}))
	defer server.Close()
	client := newTestClient(t, server)

	first, err := client.ReadRepositoryTree(context.Background(), status.Repository{Owner: "owner", Name: "first"}, "same")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.ReadRepositoryTree(context.Background(), status.Repository{Owner: "owner", Name: "second"}, "same")
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.ReadRepositoryTree(context.Background(), status.Repository{Owner: "owner", Name: "first"}, "other")
	if err != nil {
		t.Fatal(err)
	}

	if first.SHA == second.SHA || first.SHA == third.SHA || second.SHA == third.SHA {
		t.Fatalf("tree SHAs = %q, %q, %q", first.SHA, second.SHA, third.SHA)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	return newTestClientWithProbe(t, server, nil)
}

func newTestClientWithProbe(t *testing.T, server *httptest.Server, probe PushProbe) *Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := api.NewRESTClient(api.ClientOptions{
		Host:         "github.com",
		AuthToken:    "test-token",
		Transport:    rewriteTransport{target: target, base: http.DefaultTransport},
		LogIgnoreEnv: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if probe == nil {
		return New(rest)
	}
	return New(rest, probe)
}
