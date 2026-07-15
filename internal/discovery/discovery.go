package discovery

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Entry struct {
	Path string
	Type string
	SHA  string
}

type Skill struct {
	Name      string
	Namespace string
	Path      string
	TreeSHA   string
}

func (s Skill) DisplayName() string {
	if s.Namespace == "" {
		return s.Name
	}
	return s.Namespace + "/" + s.Name
}

type Result struct {
	CommitSHA string
	Skills    []Skill
}

func FromTree(commitSHA string, entries []Entry) (Result, error) {
	if commitSHA == "" {
		return Result{}, fmt.Errorf("discovery commit SHA is required")
	}
	trees := make(map[string]string)
	for _, entry := range entries {
		if entry.Type == "tree" {
			trees[entry.Path] = entry.SHA
		}
	}

	seen := make(map[string]bool)
	var skills []Skill
	for _, entry := range entries {
		if entry.Type != "blob" || path.Base(entry.Path) != "SKILL.md" {
			continue
		}
		directory := path.Dir(entry.Path)
		name, namespace, ok := match(directory)
		if !ok || seen[directory] {
			continue
		}
		treeSHA := trees[directory]
		if treeSHA == "" {
			return Result{}, fmt.Errorf("skill %q has no repository tree entry", directory)
		}
		seen[directory] = true
		skills = append(skills, Skill{
			Name: name, Namespace: namespace, Path: directory,
			TreeSHA: treeSHA,
		})
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].DisplayName() < skills[j].DisplayName()
	})
	return Result{CommitSHA: commitSHA, Skills: skills}, nil
}

func match(directory string) (string, string, bool) {
	parts := strings.Split(directory, "/")
	if directory == "." || hasHiddenPart(parts) {
		return "", "", false
	}
	if len(parts) == 1 {
		return validPair(parts[0], "")
	}
	if len(parts) == 4 && parts[0] == "plugins" && parts[2] == "skills" {
		return validPair(parts[3], parts[1])
	}
	for index, part := range parts {
		if part != "skills" || hasPart(parts[:index], "plugins") {
			continue
		}
		after := parts[index+1:]
		switch len(after) {
		case 1:
			return validPair(after[0], "")
		case 2:
			return validPair(after[1], after[0])
		}
	}
	return "", "", false
}

func validPair(name, namespace string) (string, string, bool) {
	if !namePattern.MatchString(name) || (namespace != "" && !namePattern.MatchString(namespace)) {
		return "", "", false
	}
	return name, namespace, true
}

func hasHiddenPart(parts []string) bool {
	for _, part := range parts {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func hasPart(parts []string, want string) bool {
	for _, part := range parts {
		if part == want {
			return true
		}
	}
	return false
}

func Resolve(skills []Skill, selector string) (Skill, error) {
	for _, candidate := range skills {
		if candidate.DisplayName() == selector {
			return candidate, nil
		}
	}
	var matches []Skill
	for _, candidate := range skills {
		if candidate.Name == selector {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return Skill{}, fmt.Errorf("skill %q not found", selector)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for index, candidate := range matches {
			names[index] = candidate.DisplayName()
		}
		return Skill{}, fmt.Errorf("skill name %q is ambiguous; use one of: %s", selector, strings.Join(names, ", "))
	}
}

func FindNameCollisions(skills []Skill) error {
	byName := make(map[string][]string)
	for _, candidate := range skills {
		byName[candidate.Name] = append(byName[candidate.Name], candidate.DisplayName())
	}
	var collisions []string
	for name, displayNames := range byName {
		if len(displayNames) > 1 {
			sort.Strings(displayNames)
			collisions = append(collisions, fmt.Sprintf("%s (%s)", name, strings.Join(displayNames, ", ")))
		}
	}
	if len(collisions) == 0 {
		return nil
	}
	sort.Strings(collisions)
	return fmt.Errorf("discovered skills have duplicate install names: %s", strings.Join(collisions, "; "))
}

func LooksLikePath(selector string) bool {
	selector = strings.TrimSuffix(selector, "/")
	return strings.HasSuffix(selector, "/SKILL.md") ||
		strings.HasPrefix(selector, "skills/") || strings.HasPrefix(selector, "plugins/") ||
		strings.Contains(selector, "/skills/") || strings.Contains(selector, "/plugins/") ||
		strings.Count(selector, "/") >= 2
}
