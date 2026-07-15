package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
)

const (
	FileName             = ".gh-skill-linker.json"
	CurrentSchemaVersion = 2
)

var (
	skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	shaPattern       = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

var ErrManifestChanged = errors.New("management file changed during operation")

type Skill struct {
	Repository  string `json:"repository"`
	SourcePath  string `json:"sourcePath"`
	SourceRef   string `json:"sourceRef"`
	RefSHA      string `json:"refSHA"`
	CommitSHA   string `json:"commitSHA"`
	TreeSHA     string `json:"treeSHA"`
	Destination string `json:"destination"`
}

type versionOneSkill struct {
	Repository  string `json:"repository"`
	SourcePath  string `json:"sourcePath"`
	Branch      string `json:"branch"`
	CommitSHA   string `json:"commitSHA"`
	TreeSHA     string `json:"treeSHA"`
	Destination string `json:"destination"`
}

type diskDocument struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Skills        map[string]json.RawMessage `json:"skills"`
}

type Document struct {
	SchemaVersion int              `json:"schemaVersion"`
	Skills        map[string]Skill `json:"skills"`
}

type InstalledSkill struct {
	Name string
	Path string
	Skill
}

type Store struct{}

func (store Store) ListProject(_ context.Context, projectRoot string) ([]InstalledSkill, error) {
	document, err := store.Read(projectRoot)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(document.Skills))
	for name := range document.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	installed := make([]InstalledSkill, 0, len(names))
	for _, name := range names {
		entry := document.Skills[name]
		installed = append(installed, InstalledSkill{
			Name:  name,
			Path:  filepath.Join(filepath.Clean(projectRoot), filepath.FromSlash(entry.Destination)),
			Skill: entry,
		})
	}
	return installed, nil
}

func (store Store) Advance(projectRoot, name string, expected Skill, commitSHA, treeSHA string) error {
	document, err := store.Read(projectRoot)
	if err != nil {
		return err
	}
	current, ok := document.Skills[name]
	if !ok || !reflect.DeepEqual(current, expected) {
		return ErrManifestChanged
	}
	current.CommitSHA = commitSHA
	current.RefSHA = commitSHA
	current.TreeSHA = treeSHA
	document.Skills[name] = current
	return store.Write(projectRoot, document)
}

func (store Store) Add(projectRoot, name string, expected Document, skill Skill) error {
	current, err := store.Read(projectRoot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return ErrManifestChanged
	}
	if _, exists := current.Skills[name]; exists {
		return ErrManifestChanged
	}
	current.Skills[name] = skill
	return store.Write(projectRoot, current)
}

func (store Store) Remove(projectRoot, name string, expected Skill) error {
	document, err := store.Read(projectRoot)
	if err != nil {
		return err
	}
	current, ok := document.Skills[name]
	if !ok || !reflect.DeepEqual(current, expected) {
		return ErrManifestChanged
	}
	delete(document.Skills, name)
	return store.Write(projectRoot, document)
}

func (Store) Read(projectRoot string) (Document, error) {
	manifestPath := filepath.Join(filepath.Clean(projectRoot), FileName)
	info, err := os.Lstat(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyDocument(), nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("inspect management file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Document{}, fmt.Errorf("management file must be a regular file: %s", manifestPath)
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return Document{}, fmt.Errorf("open management file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var disk diskDocument
	if err := decoder.Decode(&disk); err != nil {
		return Document{}, fmt.Errorf("decode management file: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Document{}, err
	}
	document := Document{SchemaVersion: CurrentSchemaVersion, Skills: make(map[string]Skill, len(disk.Skills))}
	switch disk.SchemaVersion {
	case 1:
		for name, raw := range disk.Skills {
			var legacy versionOneSkill
			if err := decodeStrict(raw, &legacy); err != nil {
				return Document{}, fmt.Errorf("decode managed skill %q: %w", name, err)
			}
			document.Skills[name] = Skill{
				Repository: legacy.Repository, SourcePath: legacy.SourcePath,
				SourceRef: "refs/heads/" + legacy.Branch, RefSHA: legacy.CommitSHA,
				CommitSHA: legacy.CommitSHA, TreeSHA: legacy.TreeSHA, Destination: legacy.Destination,
			}
		}
	case CurrentSchemaVersion:
		for name, raw := range disk.Skills {
			var managed Skill
			if err := decodeStrict(raw, &managed); err != nil {
				return Document{}, fmt.Errorf("decode managed skill %q: %w", name, err)
			}
			document.Skills[name] = managed
		}
	default:
		return Document{}, fmt.Errorf("unsupported management schema version %d", disk.SchemaVersion)
	}
	if err := document.Validate(); err != nil {
		return Document{}, err
	}
	return document, nil
}

func (Store) Write(projectRoot string, document Document) error {
	if document.Skills == nil {
		document.Skills = make(map[string]Skill)
	}
	if err := document.Validate(); err != nil {
		return err
	}
	projectRoot = filepath.Clean(projectRoot)
	temporary, err := os.CreateTemp(projectRoot, ".gh-skill-linker-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary management file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		_ = temporary.Close()
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set temporary management file mode: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode management file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary management file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary management file: %w", err)
	}
	manifestPath := filepath.Join(projectRoot, FileName)
	if err := os.Rename(temporaryPath, manifestPath); err != nil {
		return fmt.Errorf("activate management file: %w", err)
	}
	removeTemporary = false
	return nil
}

func (d Document) Validate() error {
	if d.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported management schema version %d", d.SchemaVersion)
	}
	for name, skill := range d.Skills {
		if err := validateSkill(name, skill); err != nil {
			return fmt.Errorf("invalid managed skill %q: %w", name, err)
		}
	}
	return nil
}

func validateSkill(name string, skill Skill) error {
	if len(name) == 0 || len(name) > 64 || !skillNamePattern.MatchString(name) {
		return fmt.Errorf("invalid skill name")
	}
	if _, reason := source.ParseRepository(skill.Repository); reason != "" {
		return fmt.Errorf("invalid repository: %s", reason)
	}
	if err := validateSourcePath(skill.SourcePath); err != nil {
		return err
	}
	if _, err := source.ParseRef(skill.SourceRef); err != nil {
		return fmt.Errorf("invalid source ref: %w", err)
	}
	if !shaPattern.MatchString(skill.RefSHA) {
		return fmt.Errorf("invalid ref SHA")
	}
	if !shaPattern.MatchString(skill.CommitSHA) {
		return fmt.Errorf("invalid commit SHA")
	}
	if !shaPattern.MatchString(skill.TreeSHA) {
		return fmt.Errorf("invalid tree SHA")
	}
	wantDestination := path.Join(".agents/skills", name)
	if skill.Destination != wantDestination {
		return fmt.Errorf("destination must be %q", wantDestination)
	}
	return nil
}

func validateSourcePath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return fmt.Errorf("invalid source path")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return fmt.Errorf("invalid source path")
	}
	return nil
}

func emptyDocument() Document {
	return Document{
		SchemaVersion: CurrentSchemaVersion,
		Skills:        make(map[string]Skill),
	}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing management data: %w", err)
	}
	return fmt.Errorf("management file contains trailing JSON value")
}

func decodeStrict(content []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}
