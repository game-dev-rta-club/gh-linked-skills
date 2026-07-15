package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/command"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/compat"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/discovery"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/gitcli"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/githubapi"
	installapp "github.com/game-dev-rta-club/gh-skill-linker/internal/install"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/manifest"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/proposal"
	publishapp "github.com/game-dev-rta-club/gh-skill-linker/internal/publish"
	pullapp "github.com/game-dev-rta-club/gh-skill-linker/internal/pull"
	pushapp "github.com/game-dev-rta-club/gh-skill-linker/internal/push"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/status"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/syncstate"
	uninstallapp "github.com/game-dev-rta-club/gh-skill-linker/internal/uninstall"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/workspace"
)

type StatusPreflight interface {
	CheckStatus(ctx context.Context) error
	CheckInstall(ctx context.Context) error
	CheckPublish(ctx context.Context) error
}

type ProjectRoot interface {
	Root(ctx context.Context) (string, error)
}

type StatusService interface {
	Inspect(ctx context.Context, projectRoot string) ([]status.Record, error)
}

type PullService interface {
	Pull(ctx context.Context, projectRoot, selector string) (pullapp.Result, error)
}

type PushService interface {
	Push(ctx context.Context, projectRoot, selector string) (pushapp.Result, error)
	PushProposal(ctx context.Context, projectRoot, selector string) (pushapp.Result, error)
}

type UninstallService interface {
	Uninstall(
		ctx context.Context,
		projectRoot, selector string,
		options uninstallapp.Options,
	) (uninstallapp.Result, error)
}

type PublishService interface {
	Publish(
		ctx context.Context,
		projectRoot, repository, selector string,
		ref source.Ref,
	) (publishapp.Result, error)
	PublishProposal(
		ctx context.Context,
		projectRoot, repository, selector string,
		ref source.Ref,
	) (publishapp.Result, error)
}

type ManagedInstaller interface {
	Install(
		ctx context.Context,
		projectRoot, repository, sourcePath string,
		ref source.Ref,
		options installapp.Options,
	) (installapp.Result, error)
	Discover(ctx context.Context, repository string, ref source.Ref) (discovery.Result, error)
	InstallAll(ctx context.Context, projectRoot, repository string, ref source.Ref) ([]installapp.Result, error)
}

type Dependencies struct {
	Preflight        StatusPreflight
	Root             ProjectRoot
	Status           StatusService
	Pull             PullService
	Push             PushService
	Uninstall        UninstallService
	Publish          PublishService
	ManagedInstaller ManagedInstaller
}

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if needsDependencies(args) {
		dependencies, err := defaultDependencies(args)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return RunWithDependencies(ctx, args, stdout, stderr, dependencies)
	}
	return RunWithDependencies(ctx, args, stdout, stderr, Dependencies{})
}

func needsDependencies(args []string) bool {
	if _, requested, _ := requestedHelp(args); requested || len(args) == 0 {
		return false
	}
	switch args[0] {
	case "install":
		_, err := parseInstallArgs(args[1:])
		return err == nil
	case "publish":
		_, err := parsePublishArgs(args[1:])
		return err == nil
	case "status":
		return len(args) == 1 || (len(args) == 2 && args[1] == "--json")
	case "pull":
		return len(args) == 2 && args[1] != ""
	case "push":
		_, err := parsePushArgs(args[1:])
		return err == nil
	case "uninstall":
		_, err := parseUninstallArgs(args[1:])
		return err == nil
	default:
		return false
	}
}

func defaultDependencies(args []string) (Dependencies, error) {
	ghRunner := command.New("gh")
	gitRunner := command.New("git")
	dependencies := Dependencies{
		Root:      gitcli.New(gitRunner),
		Uninstall: uninstallapp.NewService(manifest.Store{}, workspace.Reader{}, workspace.Writer{}),
	}
	if len(args) > 0 && args[0] == "uninstall" {
		return dependencies, nil
	}
	token, _ := auth.TokenForHost("github.com")
	if token == "" {
		return Dependencies{}, fmt.Errorf("GitHub authentication is required; run gh auth login")
	}
	authorization := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	pushGitRunner := command.NewWithEnv("git", map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0": "AUTHORIZATION: basic " + authorization,
	}, token, authorization)
	github, err := githubapi.NewDefault(gitcli.New(pushGitRunner))
	if err != nil {
		return Dependencies{}, err
	}
	dependencies.Preflight = compat.NewChecker(ghRunner)
	dependencies.Status = status.NewService(
		manifest.Store{},
		workspace.Reader{},
		github,
		gitcli.New(gitRunner),
	)
	dependencies.Pull = pullapp.NewService(
		manifest.Store{},
		workspace.Reader{},
		github,
		gitcli.New(gitRunner),
		workspace.Writer{},
	)
	dependencies.Push = pushapp.NewService(
		manifest.Store{},
		workspace.Reader{},
		github,
		gitcli.New(gitRunner),
		gitcli.New(pushGitRunner),
		proposal.NewService(github, gitcli.New(pushGitRunner)),
	)
	dependencies.Publish = publishapp.NewService(
		manifest.Store{},
		workspace.Reader{},
		github,
		gitcli.New(gitRunner),
		gitcli.New(pushGitRunner),
		proposal.NewService(github, gitcli.New(pushGitRunner)),
	)
	dependencies.ManagedInstaller = installapp.NewService(github, manifest.Store{}, workspace.Writer{})
	return dependencies, nil
}

func RunWithDependencies(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	if help, requested, err := requestedHelp(args); requested {
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		_, _ = io.WriteString(stdout, help)
		return 0
	}
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, rootHelp)
		return 2
	}
	if args[0] == "status" {
		return runStatus(ctx, args[1:], stdout, stderr, dependencies)
	}
	if args[0] == "install" {
		return runInstall(ctx, args[1:], stdout, stderr, dependencies)
	}
	if args[0] == "publish" {
		return runPublish(ctx, args[1:], stdout, stderr, dependencies)
	}
	if args[0] == "pull" {
		return runPull(ctx, args[1:], stdout, stderr, dependencies)
	}
	if args[0] == "push" {
		return runPush(ctx, args[1:], stdout, stderr, dependencies)
	}
	if args[0] == "uninstall" {
		return runUninstall(ctx, args[1:], stdout, stderr, dependencies)
	}
	_, _ = fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], rootHelp)
	return 2
}

type uninstallRequest struct {
	selector string
	force    bool
}

func parseUninstallArgs(args []string) (uninstallRequest, error) {
	request := uninstallRequest{}
	for _, argument := range args {
		switch argument {
		case "--force":
			if request.force {
				return uninstallRequest{}, fmt.Errorf("--force may be specified only once")
			}
			request.force = true
		default:
			if strings.HasPrefix(argument, "-") {
				return uninstallRequest{}, fmt.Errorf("unknown uninstall flag %q", argument)
			}
			if request.selector != "" {
				return uninstallRequest{}, fmt.Errorf("exactly one skill must be specified")
			}
			request.selector = argument
		}
	}
	if request.selector == "" {
		return uninstallRequest{}, fmt.Errorf("skill is required")
	}
	return request, nil
}

func runUninstall(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	request, err := parseUninstallArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "usage: gh skill-linker uninstall SKILL [--force]")
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if dependencies.Root == nil || dependencies.Uninstall == nil {
		_, _ = fmt.Fprintln(stderr, "uninstall dependencies are not configured")
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	result, err := dependencies.Uninstall.Uninstall(
		ctx,
		root,
		request.selector,
		uninstallapp.Options{Force: request.force},
	)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "uninstalled %s from %s\n", result.Name, result.Path)
	return 0
}

type publishRequest struct {
	repository  string
	selector    string
	ref         source.Ref
	pullRequest bool
}

func parsePublishArgs(args []string) (publishRequest, error) {
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return publishRequest{}, fmt.Errorf("repository is required in OWNER/REPO format")
	}
	if err := validateRepositoryArgument(args[0]); err != nil {
		return publishRequest{}, err
	}
	request := publishRequest{repository: args[0]}
	branch := ""
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--branch":
			if branch != "" || index+1 >= len(args) || args[index+1] == "" {
				return publishRequest{}, fmt.Errorf("--branch requires one value")
			}
			branch = args[index+1]
			index++
		case "--pr":
			if request.pullRequest {
				return publishRequest{}, fmt.Errorf("--pr may be specified only once")
			}
			request.pullRequest = true
		default:
			if strings.HasPrefix(args[index], "-") {
				return publishRequest{}, fmt.Errorf("unknown publish flag %q", args[index])
			}
			if request.selector != "" {
				return publishRequest{}, fmt.Errorf("exactly one local skill must be specified")
			}
			request.selector = args[index]
		}
	}
	if request.selector == "" {
		return publishRequest{}, fmt.Errorf("local skill is required")
	}
	if branch == "" {
		return publishRequest{}, fmt.Errorf("--branch is required")
	}
	ref, err := source.NewRef(source.BranchRef, branch)
	if err != nil {
		return publishRequest{}, err
	}
	request.ref = ref
	return request, nil
}

func runPublish(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	const usage = "usage: gh skill-linker publish OWNER/REPO SKILL --branch BRANCH [--pr]"
	request, err := parsePublishArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, usage)
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if dependencies.Preflight == nil || dependencies.Root == nil || dependencies.Publish == nil {
		_, _ = fmt.Fprintln(stderr, "publish dependencies are not configured")
		return 1
	}
	if err := dependencies.Preflight.CheckPublish(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var result publishapp.Result
	if request.pullRequest {
		result, err = dependencies.Publish.PublishProposal(
			ctx, root, request.repository, request.selector, request.ref,
		)
	} else {
		result, err = dependencies.Publish.Publish(
			ctx, root, request.repository, request.selector, request.ref,
		)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	remote := result.Repository + ":" + result.SourcePath
	if result.ProposalURL != "" {
		_, _ = fmt.Fprintf(
			stdout, "%s proposal #%d for %s: %s\n",
			result.ProposalState, result.ProposalNumber, remote, result.ProposalURL,
		)
	} else if result.Published {
		_, _ = fmt.Fprintf(stdout, "published %s to %s\n", result.SkillName, remote)
	} else {
		_, _ = fmt.Fprintf(stdout, "linked %s to existing %s\n", result.SkillName, remote)
	}
	return 0
}

func runInstall(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	const installUsage = "usage: gh skill-linker install OWNER/REPO [SKILL|PATH | --all] (--branch BRANCH | --tag TAG)"
	request, err := parseInstallArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, installUsage)
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if dependencies.Preflight == nil || dependencies.Root == nil || dependencies.ManagedInstaller == nil {
		_, _ = fmt.Fprintln(stderr, "install dependencies are not configured")
		return 1
	}
	if err := dependencies.Preflight.CheckInstall(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if request.selector == "" && !request.all {
		found, err := dependencies.ManagedInstaller.Discover(ctx, request.repository, request.ref)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(writer, "SKILL\tPATH")
		for _, candidate := range found.Skills {
			_, _ = fmt.Fprintf(writer, "%s\t%s\n", candidate.DisplayName(), candidate.Path)
		}
		_ = writer.Flush()
		return 0
	}
	if request.all {
		results, installErr := dependencies.ManagedInstaller.InstallAll(ctx, root, request.repository, request.ref)
		for _, result := range results {
			printInstallResult(stdout, result)
		}
		if installErr != nil {
			_, _ = fmt.Fprintln(stderr, installErr)
			return 1
		}
		return 0
	}
	result, err := dependencies.ManagedInstaller.Install(
		ctx,
		root,
		request.repository,
		request.selector,
		request.ref,
		installapp.Options{AcceptMovedTag: request.acceptMovedTag},
	)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	printInstallResult(stdout, result)
	return 0
}

type installRequest struct {
	repository     string
	selector       string
	ref            source.Ref
	all            bool
	acceptMovedTag bool
}

func parseInstallArgs(args []string) (installRequest, error) {
	if len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return installRequest{}, fmt.Errorf("repository is required in OWNER/REPO format")
	}
	if err := validateRepositoryArgument(args[0]); err != nil {
		return installRequest{}, err
	}
	request := installRequest{repository: args[0]}
	branch := ""
	tag := ""
	for index := 1; index < len(args); index++ {
		switch args[index] {
		case "--all":
			if request.all {
				return installRequest{}, fmt.Errorf("--all may be specified only once")
			}
			request.all = true
		case "--branch":
			if branch != "" || index+1 >= len(args) || args[index+1] == "" {
				return installRequest{}, fmt.Errorf("--branch requires one value")
			}
			branch = args[index+1]
			index++
		case "--tag":
			if tag != "" || index+1 >= len(args) || args[index+1] == "" {
				return installRequest{}, fmt.Errorf("--tag requires one value")
			}
			tag = args[index+1]
			index++
		case "--accept-moved-tag":
			if request.acceptMovedTag {
				return installRequest{}, fmt.Errorf("--accept-moved-tag may be specified only once")
			}
			request.acceptMovedTag = true
		default:
			if strings.HasPrefix(args[index], "-") {
				return installRequest{}, fmt.Errorf("unknown install flag %q", args[index])
			}
			if request.selector != "" {
				return installRequest{}, fmt.Errorf("only one skill or path may be specified")
			}
			request.selector = args[index]
		}
	}
	if (branch == "") == (tag == "") {
		return installRequest{}, fmt.Errorf("exactly one of --branch or --tag is required")
	}
	if request.all && request.selector != "" {
		return installRequest{}, fmt.Errorf("--all cannot be combined with a skill or path")
	}
	if request.acceptMovedTag && (tag == "" || request.selector == "" || request.all) {
		return installRequest{}, fmt.Errorf("--accept-moved-tag requires one skill or path and --tag")
	}
	var err error
	if branch != "" {
		request.ref, err = source.NewRef(source.BranchRef, branch)
	} else {
		request.ref, err = source.NewRef(source.TagRef, tag)
	}
	if err != nil {
		return installRequest{}, err
	}
	return request, nil
}

func validateRepositoryArgument(value string) error {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" ||
		parts[0] == "." || parts[0] == ".." || parts[0] == "~" || parts[1] == "." || parts[1] == ".." ||
		strings.TrimSpace(value) != value || strings.HasSuffix(value, ".git") || strings.ContainsAny(value, "\\?#\x00\r\n") {
		return fmt.Errorf("repository must be OWNER/REPO")
	}
	return nil
}

func printInstallResult(writer io.Writer, result installapp.Result) {
	if result.Repinned {
		previous, _ := source.ParseRef(result.PreviousRef)
		current, _ := source.ParseRef(result.SourceRef)
		_, _ = fmt.Fprintf(
			writer,
			"re-pinned %s tag: %s -> %s (%s -> %s)\n",
			result.Name,
			previous.Name,
			current.Name,
			result.PreviousRefSHA,
			result.RefSHA,
		)
	} else if result.Installed {
		_, _ = fmt.Fprintf(writer, "installed %s at %s\n", result.Name, result.Path)
	} else {
		_, _ = fmt.Fprintf(writer, "%s is already installed at %s\n", result.Name, result.Path)
	}
}

func runPush(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	request, err := parsePushArgs(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "usage: gh skill-linker push SKILL [--pr]")
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	if dependencies.Preflight == nil || dependencies.Root == nil || dependencies.Push == nil {
		_, _ = fmt.Fprintln(stderr, "push dependencies are not configured")
		return 1
	}
	if err := dependencies.Preflight.CheckStatus(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var result pushapp.Result
	if request.pullRequest {
		result, err = dependencies.Push.PushProposal(ctx, root, request.selector)
	} else {
		result, err = dependencies.Push.Push(ctx, root, request.selector)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if result.ProposalURL != "" {
		_, _ = fmt.Fprintf(
			stdout, "%s proposal #%d for %s: %s\n",
			result.ProposalState, result.ProposalNumber, result.Path, result.ProposalURL,
		)
	} else if result.Pushed {
		_, _ = fmt.Fprintf(stdout, "pushed %s to %s\n", result.Path, result.TreeSHA)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s has no source changes to push\n", result.Path)
	}
	return 0
}

type pushRequest struct {
	selector    string
	pullRequest bool
}

func parsePushArgs(args []string) (pushRequest, error) {
	request := pushRequest{}
	for _, argument := range args {
		switch argument {
		case "--pr":
			if request.pullRequest {
				return pushRequest{}, fmt.Errorf("--pr may be specified only once")
			}
			request.pullRequest = true
		default:
			if strings.HasPrefix(argument, "-") {
				return pushRequest{}, fmt.Errorf("unknown push flag %q", argument)
			}
			if request.selector != "" {
				return pushRequest{}, fmt.Errorf("exactly one skill must be specified")
			}
			request.selector = argument
		}
	}
	if request.selector == "" {
		return pushRequest{}, fmt.Errorf("skill is required")
	}
	return request, nil
}

func runPull(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	if len(args) != 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "usage: gh skill-linker pull <skill>")
		return 2
	}
	if dependencies.Preflight == nil || dependencies.Root == nil || dependencies.Pull == nil {
		_, _ = fmt.Fprintln(stderr, "pull dependencies are not configured")
		return 1
	}
	if err := dependencies.Preflight.CheckStatus(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	result, err := dependencies.Pull.Pull(ctx, root, args[0])
	if err != nil {
		if errors.Is(err, pullapp.ErrConflict) {
			for _, conflictPath := range result.ConflictPaths {
				_, _ = fmt.Fprintf(stderr, "CONFLICT (content): Merge conflict in %s\n", conflictPath)
			}
			skillName := result.SkillName
			if skillName == "" {
				skillName = args[0]
			}
			_, _ = fmt.Fprintln(stderr, "Pull completed with conflicts; fix them in the working tree.")
			_, _ = fmt.Fprintln(stderr, "After resolving, run:")
			_, _ = fmt.Fprintln(stderr, "  gh skill-linker status")
			_, _ = fmt.Fprintln(stderr, "If STATE is push, run:")
			_, _ = fmt.Fprintf(stderr, "  gh skill-linker push %s\n", skillName)
			return 1
		}
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if result.Changed {
		_, _ = fmt.Fprintf(stdout, "pulled %s to %s\n", result.Path, result.TreeSHA)
	} else {
		_, _ = fmt.Fprintf(stdout, "%s is already up to date\n", result.Path)
	}
	return 0
}

func runStatus(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	jsonOutput := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--json":
		jsonOutput = true
	default:
		_, _ = fmt.Fprintf(stderr, "usage: gh skill-linker status [--json]\n")
		return 2
	}
	if dependencies.Preflight == nil || dependencies.Root == nil || dependencies.Status == nil {
		_, _ = fmt.Fprintln(stderr, "status dependencies are not configured")
		return 1
	}
	if err := dependencies.Preflight.CheckStatus(ctx); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	root, err := dependencies.Root.Root(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	records, err := dependencies.Status.Inspect(ctx, root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(records); err != nil {
			_, _ = fmt.Fprintf(stderr, "write status JSON: %v\n", err)
			return 1
		}
	} else if err := writeStatusTable(stdout, records); err != nil {
		_, _ = fmt.Fprintf(stderr, "write status table: %v\n", err)
		return 1
	}
	for _, record := range records {
		if hasLocalChanges(record.State) && record.PushEligibility != status.Eligible &&
			(record.Proposal == nil || record.Proposal.State != proposal.Waiting) {
			_, _ = fmt.Fprintf(
				stderr,
				"warning: %s has local changes but push is %s (%s); changes remain only in this project\n",
				record.Path,
				record.PushEligibility,
				pointerValue(record.PushReason),
			)
		}
	}
	return 0
}

func writeStatusTable(writer io.Writer, records []status.Record) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "SKILL\tPATH\tSTATE\tPROPOSAL\tPULL\tPUSH"); err != nil {
		return err
	}
	for _, record := range records {
		state := "-"
		if record.State != nil {
			state = string(*record.State)
		}
		if _, err := fmt.Fprintf(
			table,
			"%s\t%s\t%s\t%s\t%s\t%s\n",
			record.SkillName,
			record.Path,
			state,
			formatProposal(record.Proposal),
			formatEligibility(record.PullEligibility, record.PullReason),
			formatEligibility(record.PushEligibility, record.PushReason),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func formatProposal(summary *proposal.Summary) string {
	if summary == nil {
		return "-"
	}
	if summary.Number == 0 {
		return string(summary.State)
	}
	return fmt.Sprintf("#%d %s", summary.Number, summary.State)
}

func formatEligibility(eligibility status.Eligibility, reason *string) string {
	if reason == nil || *reason == "" {
		return string(eligibility)
	}
	return fmt.Sprintf("%s (%s)", eligibility, *reason)
}

func hasLocalChanges(state *syncstate.State) bool {
	return state != nil && (*state == syncstate.Push || *state == syncstate.Conflict)
}

func pointerValue(value *string) string {
	if value == nil {
		return "unspecified"
	}
	return *value
}
