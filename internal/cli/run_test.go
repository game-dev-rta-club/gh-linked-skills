package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/discovery"
	installapp "github.com/game-dev-rta-club/gh-skill-linker/internal/install"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	publishapp "github.com/game-dev-rta-club/gh-skill-linker/internal/publish"
	pullapp "github.com/game-dev-rta-club/gh-skill-linker/internal/pull"
	pushapp "github.com/game-dev-rta-club/gh-skill-linker/internal/push"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/status"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/syncstate"
	uninstallapp "github.com/game-dev-rta-club/gh-skill-linker/internal/uninstall"
)

type fakePreflight struct{ err error }

func (f fakePreflight) CheckStatus(context.Context) error  { return f.err }
func (f fakePreflight) CheckInstall(context.Context) error { return f.err }
func (f fakePreflight) CheckPublish(context.Context) error { return f.err }

type fakeRoot struct {
	root string
	err  error
}

func (f fakeRoot) Root(context.Context) (string, error) { return f.root, f.err }

type fakeStatus struct {
	records []status.Record
	err     error
	calls   int
}

type fakePull struct {
	result pullapp.Result
	err    error
	calls  int
}

type fakePush struct {
	result        pushapp.Result
	err           error
	calls         int
	proposalCalls int
}

type fakePublish struct {
	root          string
	repository    string
	selector      string
	ref           source.Ref
	result        publishapp.Result
	err           error
	calls         int
	proposalCalls int
}

type fakeUninstaller struct {
	root     string
	selector string
	options  uninstallapp.Options
	result   uninstallapp.Result
	err      error
	calls    int
}

func (f *fakeUninstaller) Uninstall(
	_ context.Context,
	root, selector string,
	options uninstallapp.Options,
) (uninstallapp.Result, error) {
	f.calls++
	f.root, f.selector, f.options = root, selector, options
	return f.result, f.err
}

func (f *fakePublish) Publish(
	_ context.Context,
	root, repository, selector string,
	ref source.Ref,
) (publishapp.Result, error) {
	f.calls++
	f.root, f.repository, f.selector, f.ref = root, repository, selector, ref
	return f.result, f.err
}

func (f *fakePublish) PublishProposal(
	_ context.Context,
	root, repository, selector string,
	ref source.Ref,
) (publishapp.Result, error) {
	f.proposalCalls++
	f.root, f.repository, f.selector, f.ref = root, repository, selector, ref
	return f.result, f.err
}

type fakeManagedInstaller struct {
	root        string
	repository  string
	path        string
	ref         source.Ref
	options     installapp.Options
	result      installapp.Result
	allResults  []installapp.Result
	discovered  discovery.Result
	err         error
	calls       int
	allCalls    int
	discoveries int
}

func (f *fakeManagedInstaller) Discover(_ context.Context, repository string, ref source.Ref) (discovery.Result, error) {
	f.discoveries++
	f.repository = repository
	f.ref = ref
	return f.discovered, f.err
}

func (f *fakeManagedInstaller) InstallAll(_ context.Context, root, repository string, ref source.Ref) ([]installapp.Result, error) {
	f.allCalls++
	f.root = root
	f.repository = repository
	f.ref = ref
	return f.allResults, f.err
}

func (f *fakeManagedInstaller) Install(
	_ context.Context,
	root, repository, path string,
	ref source.Ref,
	options installapp.Options,
) (installapp.Result, error) {
	f.calls++
	f.root = root
	f.repository = repository
	f.path = path
	f.ref = ref
	f.options = options
	return f.result, f.err
}

func (f *fakePush) Push(context.Context, string, string) (pushapp.Result, error) {
	f.calls++
	return f.result, f.err
}

func (f *fakePush) PushProposal(context.Context, string, string) (pushapp.Result, error) {
	f.proposalCalls++
	return f.result, f.err
}

func (f *fakePull) Pull(context.Context, string, string) (pullapp.Result, error) {
	f.calls++
	return f.result, f.err
}

func (f *fakeStatus) Inspect(context.Context, string) ([]status.Record, error) {
	f.calls++
	return f.records, f.err
}

func TestRunHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}
	for _, want := range []string{"USAGE", "AVAILABLE COMMANDS", "EXAMPLES", "LEARN MORE", "gh skill-linker install OWNER/REPO", "publish", "uninstall"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "gh linked-skills") {
		t.Fatalf("stdout = %q, want only the gh skill-linker command", stdout.String())
	}
	if strings.Contains(stdout.String(), "\n  skills ") {
		t.Fatalf("stdout = %q, want no workflow skill command", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root help command", args: []string{"help"}, want: "AVAILABLE COMMANDS"},
		{name: "install help command", args: []string{"help", "install"}, want: "--accept-moved-tag"},
		{name: "install long flag", args: []string{"install", "--help"}, want: "repository-less installation is not supported"},
		{name: "install short flag", args: []string{"install", "-h"}, want: "repository-less installation is not supported"},
		{name: "install help after repository", args: []string{"install", "owner/repository", "--help"}, want: "repository-less installation is not supported"},
		{name: "publish help", args: []string{"publish", "--help"}, want: "unmanaged"},
		{name: "status help", args: []string{"status", "--help"}, want: "--json"},
		{name: "pull help", args: []string{"pull", "--help"}, want: "CONFLICT"},
		{name: "push help", args: []string{"push", "--help"}, want: "write permission"},
		{name: "uninstall help", args: []string{"uninstall", "--help"}, want: "--force"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := Run(context.Background(), test.args, &stdout, &stderr)

			if exitCode != 0 {
				t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), "USAGE") || !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q, want USAGE and %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestNeedsDependenciesOnlyAfterValidArguments(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"install"}, want: false},
		{args: []string{"install", "owner", "--branch", "main"}, want: false},
		{args: []string{"install", "owner/repository", "--branch", "main"}, want: true},
		{args: []string{"install", "owner/repository", "--help"}, want: false},
		{args: []string{"publish"}, want: false},
		{args: []string{"publish", "owner/repository", "sample", "--branch", "main"}, want: true},
		{args: []string{"publish", "owner/repository", "sample"}, want: false},
		{args: []string{"publish", "owner", "sample", "--branch", "main"}, want: false},
		{args: []string{"status"}, want: true},
		{args: []string{"status", "extra"}, want: false},
		{args: []string{"pull", "skill"}, want: true},
		{args: []string{"pull", "--help"}, want: false},
		{args: []string{"uninstall", "sample"}, want: true},
		{args: []string{"uninstall", "sample", "--force"}, want: true},
		{args: []string{"uninstall"}, want: false},
	}

	for _, test := range tests {
		if got := needsDependencies(test.args); got != test.want {
			t.Errorf("needsDependencies(%q) = %t, want %t", test.args, got, test.want)
		}
	}
}

func TestRunRejectsUnknownCommandAsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"unknown"}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command error", stderr.String())
	}
}

func TestRunStatusJSONIncludesNullFields(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	service := &fakeStatus{records: []status.Record{{
		SkillName:       "local",
		Path:            ".agents/skills/local",
		PullEligibility: status.Ineligible,
		PullReason:      testStringPointer("missing_source_metadata"),
		PushEligibility: status.Ineligible,
		PushReason:      testStringPointer("missing_source_metadata"),
	}}}
	dependencies := Dependencies{
		Preflight: fakePreflight{},
		Root:      fakeRoot{root: "/repo"},
		Status:    service,
	}

	exitCode := RunWithDependencies(context.Background(), []string{"status", "--json"}, &stdout, &stderr, dependencies)

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}
	want := `[{"skillName":"local","path":".agents/skills/local","sourceURL":null,"sourceRef":null,"state":null,"pullEligibility":"ineligible","pullReason":"missing_source_metadata","pushEligibility":"ineligible","pushReason":"missing_source_metadata"}]` + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunStatusTableWarnsWhenLocalChangesCannotBePushed(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	state := syncstate.Push
	service := &fakeStatus{records: []status.Record{{
		SkillName:       "sample",
		Path:            ".agents/skills/sample",
		State:           &state,
		PullEligibility: status.Eligible,
		PushEligibility: status.Ineligible,
		PushReason:      testStringPointer("repository_read_only"),
	}}}

	exitCode := RunWithDependencies(context.Background(), []string{"status"}, &stdout, &stderr, Dependencies{
		Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Status: service,
	})

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", exitCode)
	}
	for _, want := range []string{"SKILL", "sample", "push", "repository_read_only"} {
		if !strings.Contains(stdout.String()+stderr.String(), want) {
			t.Errorf("output does not contain %q; stdout=%q stderr=%q", want, stdout.String(), stderr.String())
		}
	}
	if !strings.Contains(stderr.String(), "local changes") {
		t.Fatalf("stderr = %q, want local changes warning", stderr.String())
	}
}

func TestRunStatusTableShowsProposalAsSeparateColumn(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	state := syncstate.Push
	service := &fakeStatus{records: []status.Record{{
		SkillName: "sample", Path: ".agents/skills/sample", State: &state,
		PullEligibility: status.Eligible, PushEligibility: status.Ineligible,
		PushReason: testStringPointer("open_proposal"),
		Proposal: &proposal.Summary{
			State: proposal.Waiting, Number: 42, URL: "https://github.com/owner/repo/pull/42",
		},
	}}}

	exitCode := RunWithDependencies(
		context.Background(), []string{"status"}, &stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Status: service},
	)

	if exitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{"PROPOSAL", "#42 waiting", "open_proposal"} {
		if !strings.Contains(stdout.String()+stderr.String(), want) {
			t.Errorf("output = %q / %q, want %q", stdout.String(), stderr.String(), want)
		}
	}
}

func TestRunStatusStopsAfterPreflightFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	service := &fakeStatus{}

	exitCode := RunWithDependencies(context.Background(), []string{"status"}, &stdout, &stderr, Dependencies{
		Preflight: fakePreflight{err: errors.New("upgrade gh")},
		Root:      fakeRoot{root: "/repo"},
		Status:    service,
	})

	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if service.calls != 0 {
		t.Fatalf("status calls = %d, want 0", service.calls)
	}
	if !strings.Contains(stderr.String(), "upgrade gh") {
		t.Fatalf("stderr = %q, want preflight error", stderr.String())
	}
}

func TestRunStatusRejectsUnexpectedArgument(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunWithDependencies(context.Background(), []string{"status", "extra"}, &stdout, &stderr, Dependencies{})

	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
}

func testStringPointer(value string) *string { return &value }

func TestRunPullReportsAppliedTree(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	puller := &fakePull{result: pullapp.Result{Path: ".agents/skills/sample", TreeSHA: "new-tree", Changed: true}}

	exitCode := RunWithDependencies(context.Background(), []string{"pull", "sample"}, &stdout, &stderr, Dependencies{
		Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Pull: puller,
	})

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "new-tree") {
		t.Fatalf("stdout = %q, want new tree SHA", stdout.String())
	}
}

func TestRunPullRejectsMissingSelector(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunWithDependencies(context.Background(), []string{"pull"}, &stdout, &stderr, Dependencies{})

	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
}

func TestRunPullReportsManualConflictAsOperationFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	puller := &fakePull{
		result: pullapp.Result{
			SkillName:     "sample",
			Path:          ".agents/skills/sample",
			Changed:       true,
			Conflict:      true,
			ConflictPaths: []string{".agents/skills/sample/SKILL.md", ".agents/skills/sample/notes.md"},
		},
		err: pullapp.ErrConflict,
	}

	exitCode := RunWithDependencies(context.Background(), []string{"pull", "sample"}, &stdout, &stderr, Dependencies{
		Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Pull: puller,
	})

	if exitCode != 1 {
		t.Fatalf("Run() exit code = %d, want 1", exitCode)
	}
	for _, want := range []string{
		"CONFLICT (content): Merge conflict in .agents/skills/sample/SKILL.md",
		"CONFLICT (content): Merge conflict in .agents/skills/sample/notes.md",
		"Pull completed with conflicts; fix them in the working tree.",
		"gh skill-linker status",
		"gh skill-linker push sample",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunPushReportsPushedTree(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	pusher := &fakePush{result: pushapp.Result{Path: ".agents/skills/sample", TreeSHA: "new-tree", Pushed: true}}

	exitCode := RunWithDependencies(context.Background(), []string{"push", "sample"}, &stdout, &stderr, Dependencies{
		Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Push: pusher,
	})

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "new-tree") {
		t.Fatalf("stdout = %q, want pushed tree", stdout.String())
	}
}

func TestRunPushProposalReportsPullRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	pusher := &fakePush{result: pushapp.Result{
		Path: ".agents/skills/sample", TreeSHA: "new-tree", Proposed: true,
		ProposalState: "created", ProposalNumber: 42,
		ProposalURL: "https://github.com/owner/repo/pull/42",
	}}

	exitCode := RunWithDependencies(
		context.Background(), []string{"push", "sample", "--pr"}, &stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Push: pusher},
	)

	if exitCode != 0 || pusher.proposalCalls != 1 || pusher.calls != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d direct=%d proposal=%d stderr=%q", exitCode, pusher.calls, pusher.proposalCalls, stderr.String())
	}
	for _, want := range []string{"created", "#42", "https://github.com/owner/repo/pull/42"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunPushRejectsInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"push"}, {"push", "sample", "extra"}, {"push", "sample", "--pr", "--pr"}, {"push", "--unknown", "sample"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		pusher := &fakePush{}

		exitCode := RunWithDependencies(context.Background(), args, &stdout, &stderr, Dependencies{Push: pusher})

		if exitCode != 2 || pusher.calls != 0 || pusher.proposalCalls != 0 {
			t.Errorf("args=%q exit=%d direct=%d proposal=%d", args, exitCode, pusher.calls, pusher.proposalCalls)
		}
	}
}

func TestRunPushRejectsMissingSelector(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunWithDependencies(context.Background(), []string{"push"}, &stdout, &stderr, Dependencies{})

	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
}

func TestRunUninstallPassesSelectorAndForce(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	service := &fakeUninstaller{result: uninstallapp.Result{Name: "sample", Path: ".agents/skills/sample"}}

	exitCode := RunWithDependencies(context.Background(), []string{"uninstall", "sample", "--force"}, &stdout, &stderr, Dependencies{
		Root: fakeRoot{root: "/repo"}, Uninstall: service,
	})

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	if service.calls != 1 || service.root != "/repo" || service.selector != "sample" || !service.options.Force {
		t.Fatalf("uninstall request = %#v", service)
	}
	if !strings.Contains(stdout.String(), "uninstalled sample from .agents/skills/sample") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunUninstallRejectsInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"uninstall"},
		{"uninstall", "sample", "extra"},
		{"uninstall", "sample", "--force", "--force"},
		{"uninstall", "sample", "--unknown"},
		{"uninstall", "--force"},
	}
	for _, args := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		service := &fakeUninstaller{}

		exitCode := RunWithDependencies(context.Background(), args, &stdout, &stderr, Dependencies{
			Root: fakeRoot{root: "/repo"}, Uninstall: service,
		})

		if exitCode != 2 || service.calls != 0 {
			t.Errorf("args=%q exit=%d calls=%d stderr=%q", args, exitCode, service.calls, stderr.String())
		}
	}
}

func TestRunPublishPassesExplicitRepositorySkillAndBranch(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	publisher := &fakePublish{result: publishapp.Result{
		SkillName: "sample", Path: ".agents/skills/sample", Repository: "nikollson/skills",
		SourcePath: "skills/sample", TreeSHA: "tree", Published: true,
	}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"publish", "nikollson/skills", "sample", "--branch", "main"},
		&stdout,
		&stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Publish: publisher},
	)

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", exitCode, stderr.String())
	}
	if publisher.calls != 1 || publisher.root != "/repo" || publisher.repository != "nikollson/skills" ||
		publisher.selector != "sample" || publisher.ref.FullName != "refs/heads/main" {
		t.Fatalf("publish request = %#v", publisher)
	}
	for _, want := range []string{"published", "sample", "nikollson/skills", "skills/sample"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunPublishReportsExistingRemoteLink(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	publisher := &fakePublish{result: publishapp.Result{
		SkillName: "sample", Path: ".agents/skills/sample", Repository: "nikollson/skills",
		SourcePath: "skills/sample", Published: false,
	}}

	exitCode := RunWithDependencies(
		context.Background(), []string{"publish", "nikollson/skills", "sample", "--branch", "main"},
		&stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Publish: publisher},
	)

	if exitCode != 0 || !strings.Contains(stdout.String(), "linked") || !strings.Contains(stdout.String(), "existing") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunPublishProposalReportsPullRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	publisher := &fakePublish{result: publishapp.Result{
		SkillName: "sample", Path: ".agents/skills/sample", Repository: "nikollson/skills",
		SourcePath: "skills/sample", Proposed: true, ProposalState: "created",
		ProposalNumber: 42, ProposalURL: "https://github.com/nikollson/skills/pull/42",
	}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"publish", "nikollson/skills", "sample", "--branch", "main", "--pr"},
		&stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Publish: publisher},
	)

	if exitCode != 0 || publisher.proposalCalls != 1 || publisher.calls != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d direct=%d proposal=%d stderr=%q", exitCode, publisher.calls, publisher.proposalCalls, stderr.String())
	}
	for _, want := range []string{"created", "#42", "https://github.com/nikollson/skills/pull/42"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunPublishRejectsInvalidArgumentsBeforeService(t *testing.T) {
	tests := [][]string{
		{"publish"},
		{"publish", "owner/repo"},
		{"publish", "owner/repo", "sample"},
		{"publish", "owner/repo", "sample", "--tag", "v1"},
		{"publish", "owner/repo", "sample", "--branch", "main", "extra"},
		{"publish", "owner/repo", "sample", "--branch", "main", "--branch", "next"},
		{"publish", "owner/repo", "sample", "--branch", "main", "--pr", "--pr"},
		{"publish", "owner/repo", "sample", "--branch=main"},
		{"publish", "owner", "sample", "--branch", "main"},
	}
	for _, args := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		publisher := &fakePublish{}

		exitCode := RunWithDependencies(context.Background(), args, &stdout, &stderr, Dependencies{
			Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, Publish: publisher,
		})

		if exitCode != 2 || publisher.calls != 0 {
			t.Errorf("args=%q exit=%d calls=%d stderr=%q", args, exitCode, publisher.calls, stderr.String())
		}
	}
}

func TestRunInstallPassesPositionalSourcePathToService(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installer := &fakeManagedInstaller{result: installapp.Result{Name: "sample", Path: ".agents/skills/sample", Installed: true}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"install", "nikollson/sample-skills", "skills/sample", "--branch", "main"},
		&stdout,
		&stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer},
	)

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr=%q", exitCode, stderr.String())
	}
	if installer.calls != 1 || installer.root != "/repo" || installer.repository != "nikollson/sample-skills" ||
		installer.path != "skills/sample" || installer.ref.FullName != "refs/heads/main" {
		t.Fatalf("install request = %#v", installer)
	}
	if !strings.Contains(stdout.String(), ".agents/skills/sample") {
		t.Fatalf("stdout = %q, want installed path", stdout.String())
	}
}

func TestRunInstallPassesTagAndMovedTagApprovalToService(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installer := &fakeManagedInstaller{result: installapp.Result{
		Name: "sample", Path: ".agents/skills/sample", Repinned: true,
		PreviousRef: "refs/tags/v1.0.0", PreviousRefSHA: "old-ref",
		SourceRef: "refs/tags/v2.0.0", RefSHA: "new-ref",
	}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"install", "nikollson/sample-skills", "skills/sample", "--tag", "v2.0.0", "--accept-moved-tag"},
		&stdout,
		&stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer},
	)

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if installer.ref.Kind != source.TagRef || installer.ref.Name != "v2.0.0" || !installer.options.AcceptMovedTag {
		t.Fatalf("install request = %#v", installer)
	}
	for _, want := range []string{"re-pinned sample tag", "v1.0.0", "v2.0.0", "old-ref", "new-ref"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunInstallWithoutSelectorListsDiscoveredSkills(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installer := &fakeManagedInstaller{discovered: discovery.Result{Skills: []discovery.Skill{
		{Name: "alpha", Path: "skills/alpha"},
		{Name: "review", Namespace: "team", Path: "skills/team/review"},
	}}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"install", "nikollson/sample-skills", "--branch", "main"},
		&stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer},
	)

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	for _, want := range []string{"SKILL", "PATH", "alpha", "skills/alpha", "team/review", "skills/team/review"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if installer.discoveries != 1 || installer.calls != 0 || installer.allCalls != 0 {
		t.Fatalf("installer = %#v", installer)
	}
}

func TestRunInstallAllPrintsEveryResult(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installer := &fakeManagedInstaller{allResults: []installapp.Result{
		{Name: "alpha", Path: ".agents/skills/alpha", Installed: true},
		{Name: "beta", Path: ".agents/skills/beta", Installed: false},
	}}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"install", "nikollson/sample-skills", "--all", "--branch", "main"},
		&stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer},
	)

	if exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "installed alpha") || !strings.Contains(stdout.String(), "beta is already installed") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if installer.allCalls != 1 || installer.ref.FullName != "refs/heads/main" {
		t.Fatalf("installer = %#v", installer)
	}
}

func TestRunInstallAllPrintsPartialSuccessBeforeError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	installer := &fakeManagedInstaller{
		allResults: []installapp.Result{{Name: "alpha", Path: ".agents/skills/alpha", Installed: true}},
		err:        errors.New("install beta: failed"),
	}

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"install", "nikollson/sample-skills", "--all", "--branch", "main"},
		&stdout, &stderr,
		Dependencies{Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer},
	)

	if exitCode != 1 || !strings.Contains(stdout.String(), "installed alpha") || !strings.Contains(stderr.String(), "install beta: failed") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunInstallRejectsInvalidSourceArguments(t *testing.T) {
	tests := [][]string{
		{"install", "nikollson/sample-skills", "skills/sample"},
		{"install", "nikollson/sample-skills", "skills/sample", "--branch", "main", "--tag", "v1.0.0"},
		{"install", "nikollson/sample-skills", "skills/sample", "--tag", "v1.0.0", "--tag", "v2.0.0"},
		{"install", "nikollson/sample-skills", "skills/sample", "--branch", "main", "--accept-moved-tag"},
		{"install", "nikollson/sample-skills", "--tag", "v1.0.0", "--accept-moved-tag"},
		{"install", "nikollson/sample-skills", "--all", "--tag", "v1.0.0", "--accept-moved-tag"},
		{"install", "nikollson/sample-skills", "skills/sample", "--all", "--branch", "main"},
		{"install", "nikollson/sample-skills", "skills/sample", "--branch", "main", "--branch", "next"},
		{"install", "nikollson/sample-skills", "--path", "skills/sample", "--branch", "main"},
		{"install", "nikollson/sample-skills", "skills/sample", "--branch=main"},
		{"install", "nikollson/sample-skills", "skills/sample", "main", "--branch"},
	}
	for _, args := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		installer := &fakeManagedInstaller{}
		exitCode := RunWithDependencies(context.Background(), args, &stdout, &stderr, Dependencies{
			Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer,
		})
		if exitCode != 2 {
			t.Errorf("RunWithDependencies(%q) exit = %d, want 2", args, exitCode)
		}
		if installer.calls != 0 {
			t.Errorf("RunWithDependencies(%q) called installer", args)
		}
	}
}

func TestRunInstallRequiresExplicitRepository(t *testing.T) {
	tests := [][]string{
		{"install"},
		{"install", "--branch", "main"},
		{"install", "owner", "--branch", "main"},
		{"install", "owner/", "--branch", "main"},
		{"install", "/repository", "--branch", "main"},
		{"install", "owner/repository/extra", "--branch", "main"},
		{"install", "./skills", "--branch", "main"},
		{"install", "../skills", "--branch", "main"},
		{"install", "~/skills", "--branch", "main"},
		{"install", "owner/repository.git", "--branch", "main"},
		{"install", " owner/repository", "--branch", "main"},
	}

	for _, args := range tests {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		installer := &fakeManagedInstaller{}

		exitCode := RunWithDependencies(context.Background(), args, &stdout, &stderr, Dependencies{
			Preflight: fakePreflight{}, Root: fakeRoot{root: "/repo"}, ManagedInstaller: installer,
		})

		if exitCode != 2 {
			t.Errorf("RunWithDependencies(%q) exit = %d, want 2; stderr = %q", args, exitCode, stderr.String())
		}
		if !strings.Contains(stderr.String(), "OWNER/REPO") {
			t.Errorf("RunWithDependencies(%q) stderr = %q, want OWNER/REPO guidance", args, stderr.String())
		}
		if installer.calls != 0 || installer.allCalls != 0 || installer.discoveries != 0 {
			t.Errorf("RunWithDependencies(%q) called installer: %#v", args, installer)
		}
	}
}

func TestRunRejectsRemovedSkillsCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := RunWithDependencies(
		context.Background(),
		[]string{"skills", "install", "--agent", "codex"},
		&stdout,
		&stderr,
		Dependencies{},
	)

	if exitCode != 2 {
		t.Fatalf("Run() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown command "skills"`) {
		t.Fatalf("stderr = %q, want removed command error", stderr.String())
	}
}
