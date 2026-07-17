package cli

import "fmt"

const rootHelp = `Keep project Agent Skills linked to GitHub repositories.

Every installed skill records its source repository, path, branch or tag, and
last synchronized revision. Branch sources can pull and push changes; tag
sources remain fixed and read-only.

USAGE
  gh skill-linker <command> [flags]

AVAILABLE COMMANDS
  install   Discover and install managed skills from a repository
  publish   Publish or propose an unmanaged local skill to a repository
  status    Show visible skills and managed synchronization state
  pull      Pull one managed skill from its source branch
  push      Push one managed skill directly or propose it with a pull request
  uninstall Remove one managed skill from the current project

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  # List skills available in a repository
  $ gh skill-linker install OWNER/REPO --branch BRANCH

  # Install one skill and inspect its synchronization state
  $ gh skill-linker install OWNER/REPO SKILL --branch BRANCH

  # Publish a new local skill and begin managing it
  $ gh skill-linker publish OWNER/REPO SKILL --branch BRANCH

  # Propose a new skill through a pull request
  $ gh skill-linker publish OWNER/REPO SKILL --branch BRANCH --pr

  # Install a fixed, read-only tag snapshot
  $ gh skill-linker install OWNER/REPO SKILL --tag TAG
  $ gh skill-linker status

  # Synchronize one managed skill
  $ gh skill-linker pull SKILL
  $ gh skill-linker push SKILL

  # Propose local changes without writing directly to the source branch
  $ gh skill-linker push SKILL --pr

  # Remove one managed skill from this project
  $ gh skill-linker uninstall SKILL

LEARN MORE
  Use gh skill-linker <command> --help for more information about a command.
`

const publishHelp = `Publish one unmanaged project Agent Skill to an existing GitHub repository.

The repository, local skill, and source branch are required. The local skill
must exist at .agents/skills/<name> and must not already be managed. Publish
writes it to skills/<name> in the repository and records the source in
.gh-skill-linker.json.

An empty repository is initialized with the explicit branch. In a non-empty
repository, the branch must already exist. Existing different content is never
overwritten. Exact existing content is linked without creating a commit.

Use --pr to create or update one pull request for the skill. The manifest is
not changed until the pull request is merged. Rerun the same command after the
merge to link the skill. If local work continued meanwhile, the merged revision
is linked first and the newer local changes remain available for push --pr.

USAGE
  gh skill-linker publish OWNER/REPO SKILL --branch BRANCH [--pr]

ARGUMENTS
  OWNER/REPO   Existing GitHub repository that will own the skill
  SKILL        Unmanaged local skill name or .agents/skills/<name> path
  BRANCH       Branch used by later pull and push operations

FLAGS
  --branch string   Source branch
  --pr              Create or update a pull request
  -h, --help            Show help for command

EXAMPLES
  $ gh skill-linker publish OWNER/my-skill my-skill --branch main
  $ gh skill-linker publish OWNER/my-skill my-skill --branch main --pr

LEARN MORE
  Run gh skill-linker status after publishing.
`

const installHelp = `Discover, install, and re-pin Agent Skills from an explicit GitHub repository.

The repository and exactly one source branch or tag are required. Local and
repository-less installation is not supported.
Installed skills are copied to .agents/skills and linked to their source in
.gh-skill-linker.json. Without SKILL, PATH, or --all, the command lists
discovered skills without installing them.

Branch-backed skills support later pull and push operations. Tag-backed skills
are fixed, read-only snapshots. Re-run install with the same repository and path
and a different tag to re-pin a clean tag-backed skill. Local changes are never
merged or discarded during re-pin.

USAGE
  gh skill-linker install OWNER/REPO --branch BRANCH
  gh skill-linker install OWNER/REPO SKILL --branch BRANCH
  gh skill-linker install OWNER/REPO PATH --branch BRANCH
  gh skill-linker install OWNER/REPO --all --branch BRANCH
  gh skill-linker install OWNER/REPO SKILL --tag TAG
  gh skill-linker install OWNER/REPO PATH --tag TAG
  gh skill-linker install OWNER/REPO --all --tag TAG

ARGUMENTS
  OWNER/REPO   GitHub repository that owns the skill
  SKILL        Discovered skill name, or namespace/name when ambiguous
  PATH         Exact skill directory or SKILL.md path in the repository
  BRANCH       Branch used by later pull and push operations
  TAG          Fixed tag snapshot; pull and push are unavailable

FLAGS
      --all             Install every discovered skill
      --branch string   Source branch (conflicts with --tag)
      --tag string      Exact source tag (conflicts with --branch)
      --accept-moved-tag
                        Accept a changed ref SHA when reinstalling the same tag
  -h, --help            Show help for command

EXAMPLES
  # List available skills
  $ gh skill-linker install obra/superpowers --branch main

  # Install by name or exact path
  $ gh skill-linker install obra/superpowers brainstorming --branch main
  $ gh skill-linker install obra/superpowers skills/brainstorming --branch main

  # Install a fixed release or re-pin it to another tag
  $ gh skill-linker install owner/skills skills/review --tag v1.0.0
  $ gh skill-linker install owner/skills skills/review --tag v2.0.0

  # Install every discovered skill
  $ gh skill-linker install obra/superpowers --all --branch main

LEARN MORE
  Run gh skill-linker status after installation.
`

const statusHelp = `Show Agent Skills visible from project, user, and system scopes.

PROVIDER identifies skill-linker, gh-skill, codex-plugin, local, and
codex-system skills. Managed skills retain synchronization, proposal, pull, and
push status. Other skills report presence or plugin enablement without checking
for updates. Exact paths remain available in --json output.

An unavailable optional inventory is reported as a warning. Codex is not a
required dependency when no plugin inventory is available.

USAGE
  gh skill-linker status [--json]

FLAGS
      --json   Write machine-readable JSON
  -h, --help   Show help for command

EXAMPLES
  $ gh skill-linker status
  $ gh skill-linker status --json
`

const pullHelp = `Pull one managed skill from its recorded repository and branch.

Local and remote changes are merged into the project working tree. A content
CONFLICT is written into the affected files with Git-style conflict markers.
Resolve those files manually, then run status before pushing.

USAGE
  gh skill-linker pull SKILL

ARGUMENTS
  SKILL   Managed skill name or project-relative path

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  $ gh skill-linker pull brainstorming
  $ gh skill-linker status
`

const pushHelp = `Push one managed skill to its recorded repository and branch.

Push requires repository write permission and a remote branch that has not
changed since the last synchronization. Use --pr to create or update one pull
request for this skill instead of writing directly to the source branch.

Later local changes update the same open pull request. If the source branch
changed, pull and resolve it first, then rerun push --pr. Direct push is refused
while a managed pull request remains open.

USAGE
  gh skill-linker push SKILL [--pr]

ARGUMENTS
  SKILL   Managed skill name or project-relative path

FLAGS
      --pr   Create or update a pull request

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  $ gh skill-linker status
  $ gh skill-linker push brainstorming
  $ gh skill-linker push brainstorming --pr
`

const uninstallHelp = `Remove one managed Agent Skill from the current project.

The skill directory and its entry in .gh-skill-linker.json are removed. The
source repository is never changed. Local changes are rejected by default; use
--force only when those changes may be discarded. A missing skill directory is
cleaned up by removing its stale management entry. GitHub authentication and a
network connection are not required.

USAGE
  gh skill-linker uninstall SKILL [--force]

ARGUMENTS
  SKILL   Managed skill name or project-relative path

FLAGS
      --force   Discard local changes while uninstalling
  -h, --help    Show help for command

EXAMPLES
  $ gh skill-linker uninstall brainstorming
  $ gh skill-linker uninstall .agents/skills/brainstorming --force
`

func requestedHelp(args []string) (string, bool, error) {
	if len(args) == 1 && isHelpFlag(args[0]) {
		return rootHelp, true, nil
	}
	if len(args) > 0 && args[0] == "help" {
		if len(args) == 1 {
			return rootHelp, true, nil
		}
		if len(args) != 2 {
			return "", true, fmt.Errorf("usage: gh skill-linker help [command]")
		}
		help, ok := commandHelp(args[1])
		if !ok {
			return "", true, fmt.Errorf("unknown help topic %q", args[1])
		}
		return help, true, nil
	}
	if len(args) > 1 {
		help, ok := commandHelp(args[0])
		if ok {
			for _, argument := range args[1:] {
				if isHelpFlag(argument) {
					return help, true, nil
				}
			}
		}
	}
	return "", false, nil
}

func commandHelp(command string) (string, bool) {
	switch command {
	case "install":
		return installHelp, true
	case "publish":
		return publishHelp, true
	case "status":
		return statusHelp, true
	case "pull":
		return pullHelp, true
	case "push":
		return pushHelp, true
	case "uninstall":
		return uninstallHelp, true
	default:
		return "", false
	}
}

func isHelpFlag(value string) bool {
	return value == "--help" || value == "-h"
}
