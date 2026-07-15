package cli

import "fmt"

const rootHelp = `Keep project Agent Skills linked to GitHub repositories.

Every installed skill records its source repository, path, branch or tag, and
last synchronized revision. Branch sources can pull and push changes; tag
sources remain fixed and read-only.

USAGE
  gh linked-skills <command> [flags]

AVAILABLE COMMANDS
  install   Discover and install managed skills from a repository
  publish   Publish an unmanaged local skill to a repository
  status    Show project skill synchronization state
  pull      Pull one managed skill from its source branch
  push      Push one managed skill to its source branch

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  # List skills available in a repository
  $ gh linked-skills install OWNER/REPO --branch BRANCH

  # Install one skill and inspect its synchronization state
  $ gh linked-skills install OWNER/REPO SKILL --branch BRANCH

  # Publish a new local skill and begin managing it
  $ gh linked-skills publish OWNER/REPO SKILL --branch BRANCH

  # Install a fixed, read-only tag snapshot
  $ gh linked-skills install OWNER/REPO SKILL --tag TAG
  $ gh linked-skills status

  # Synchronize one managed skill
  $ gh linked-skills pull SKILL
  $ gh linked-skills push SKILL

LEARN MORE
  Use gh linked-skills <command> --help for more information about a command.
`

const publishHelp = `Publish one unmanaged project Agent Skill to an existing GitHub repository.

The repository, local skill, and source branch are required. The local skill
must exist at .agents/skills/<name> and must not already be managed. Publish
writes it to skills/<name> in the repository and records the source in
.gh-linked-skills.json.

An empty repository is initialized with the explicit branch. In a non-empty
repository, the branch must already exist. Existing different content is never
overwritten. Exact existing content is linked without creating a commit.

USAGE
  gh linked-skills publish OWNER/REPO SKILL --branch BRANCH

ARGUMENTS
  OWNER/REPO   Existing GitHub repository that will own the skill
  SKILL        Unmanaged local skill name or .agents/skills/<name> path
  BRANCH       Branch used by later pull and push operations

FLAGS
      --branch string   Source branch
  -h, --help            Show help for command

EXAMPLES
  $ gh linked-skills publish nikollson/agent-skills my-skill --branch main

LEARN MORE
  Run gh linked-skills status after publishing.
`

const installHelp = `Discover, install, and re-pin Agent Skills from an explicit GitHub repository.

The repository and exactly one source branch or tag are required. Local and
repository-less installation is not supported.
Installed skills are copied to .agents/skills and linked to their source in
.gh-linked-skills.json. Without SKILL, PATH, or --all, the command lists
discovered skills without installing them.

Branch-backed skills support later pull and push operations. Tag-backed skills
are fixed, read-only snapshots. Re-run install with the same repository and path
and a different tag to re-pin a clean tag-backed skill. Local changes are never
merged or discarded during re-pin.

USAGE
  gh linked-skills install OWNER/REPO --branch BRANCH
  gh linked-skills install OWNER/REPO SKILL --branch BRANCH
  gh linked-skills install OWNER/REPO PATH --branch BRANCH
  gh linked-skills install OWNER/REPO --all --branch BRANCH
  gh linked-skills install OWNER/REPO SKILL --tag TAG
  gh linked-skills install OWNER/REPO PATH --tag TAG
  gh linked-skills install OWNER/REPO --all --tag TAG

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
  $ gh linked-skills install obra/superpowers --branch main

  # Install by name or exact path
  $ gh linked-skills install obra/superpowers brainstorming --branch main
  $ gh linked-skills install obra/superpowers skills/brainstorming --branch main

  # Install a fixed release or re-pin it to another tag
  $ gh linked-skills install owner/skills skills/review --tag v1.0.0
  $ gh linked-skills install owner/skills skills/review --tag v2.0.0

  # Install every discovered skill
  $ gh linked-skills install obra/superpowers --all --branch main

LEARN MORE
  Run gh linked-skills status after installation.
`

const statusHelp = `Show synchronization and operation eligibility for project Agent Skills.

When synchronization state can be calculated, the table reports clean, pull,
push, or conflict.
Local changes that cannot be pushed are reported as warnings.
Tag-backed skills report pull and push as ineligible.

USAGE
  gh linked-skills status [--json]

FLAGS
      --json   Write machine-readable JSON
  -h, --help   Show help for command

EXAMPLES
  $ gh linked-skills status
  $ gh linked-skills status --json
`

const pullHelp = `Pull one managed skill from its recorded repository and branch.

Local and remote changes are merged into the project working tree. A content
CONFLICT is written into the affected files with Git-style conflict markers.
Resolve those files manually, then run status before pushing.

USAGE
  gh linked-skills pull SKILL

ARGUMENTS
  SKILL   Managed skill name or project-relative path

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  $ gh linked-skills pull brainstorming
  $ gh linked-skills status
`

const pushHelp = `Push one managed skill to its recorded repository and branch.

Push requires repository write permission and a remote branch that has not
changed since the last synchronization. Use status before pushing local changes.

USAGE
  gh linked-skills push SKILL

ARGUMENTS
  SKILL   Managed skill name or project-relative path

INHERITED FLAGS
  -h, --help   Show help for command

EXAMPLES
  $ gh linked-skills status
  $ gh linked-skills push brainstorming
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
			return "", true, fmt.Errorf("usage: gh linked-skills help [command]")
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
	default:
		return "", false
	}
}

func isHelpFlag(value string) bool {
	return value == "--help" || value == "-h"
}
