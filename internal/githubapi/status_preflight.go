package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/source"
	"github.com/game-dev-rta-club/gh-skill-linker/internal/status"
)

const (
	statusPreflightChunkRefs = 32
	annotatedTagPeelLimit    = 16
)

type graphQLTarget struct {
	TypeName string         `json:"__typename"`
	OID      string         `json:"oid"`
	Target   *graphQLTarget `json:"target"`
}

type graphQLRef struct {
	Target graphQLTarget `json:"target"`
}

type preflightRepository struct {
	repository     source.Repository
	refs           []string
	readPermission bool
}

type preflightPiece struct {
	repository     source.Repository
	refs           []string
	readPermission bool
}

type preflightChunk struct {
	pieces []preflightPiece
}

type graphQLChunkMetadata struct {
	repositories map[string]preflightPiece
	refs         map[string]map[string]string
}

func (c *Client) ReadStatusPreflight(
	ctx context.Context,
	requests []status.PreflightRequest,
) map[string]status.PreflightResult {
	repositories := normalizePreflightRequests(requests)
	results := initializePreflightResults(repositories)
	if c.graphql == nil {
		err := errors.New("GitHub GraphQL client is unavailable")
		for _, chunk := range chunkPreflightRepositories(repositories) {
			markPreflightChunkError(chunk, results, err)
		}
		return results
	}
	for _, chunk := range chunkPreflightRepositories(repositories) {
		c.readStatusPreflightChunk(ctx, chunk, results)
	}
	return results
}

func normalizePreflightRequests(requests []status.PreflightRequest) []preflightRepository {
	type builder struct {
		repository     source.Repository
		refs           map[string]struct{}
		readPermission bool
	}
	builders := make(map[string]*builder)
	for _, request := range requests {
		key := preflightRepositoryKey(request.Repository)
		item, ok := builders[key]
		if !ok {
			item = &builder{repository: request.Repository, refs: make(map[string]struct{})}
			builders[key] = item
		}
		for _, ref := range request.Refs {
			item.refs[ref] = struct{}{}
		}
		item.readPermission = item.readPermission || request.ReadPermission
	}
	keys := make([]string, 0, len(builders))
	for key := range builders {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	repositories := make([]preflightRepository, 0, len(keys))
	for _, key := range keys {
		item := builders[key]
		refs := make([]string, 0, len(item.refs))
		for ref := range item.refs {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		repositories = append(repositories, preflightRepository{
			repository: item.repository, refs: refs, readPermission: item.readPermission,
		})
	}
	return repositories
}

func initializePreflightResults(repositories []preflightRepository) map[string]status.PreflightResult {
	results := make(map[string]status.PreflightResult, len(repositories))
	for _, repository := range repositories {
		results[preflightRepositoryKey(repository.repository)] = status.PreflightResult{
			Refs: make(map[string]status.PreflightRefResult, len(repository.refs)),
		}
	}
	return results
}

func chunkPreflightRepositories(repositories []preflightRepository) []preflightChunk {
	chunks := make([]preflightChunk, 0)
	current := preflightChunk{}
	refCount := 0
	for _, repository := range repositories {
		remaining := repository.refs
		firstPiece := true
		for len(remaining) > 0 {
			if refCount == statusPreflightChunkRefs {
				chunks = append(chunks, current)
				current = preflightChunk{}
				refCount = 0
			}
			available := statusPreflightChunkRefs - refCount
			count := min(len(remaining), available)
			piece := preflightPiece{
				repository:     repository.repository,
				refs:           append([]string(nil), remaining[:count]...),
				readPermission: firstPiece && repository.readPermission,
			}
			current.pieces = append(current.pieces, piece)
			refCount += count
			remaining = remaining[count:]
			firstPiece = false
		}
	}
	if len(current.pieces) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func (c *Client) readStatusPreflightChunk(
	ctx context.Context,
	chunk preflightChunk,
	results map[string]status.PreflightResult,
) {
	query, variables, metadata := buildPreflightQuery(chunk)
	data := make(map[string]json.RawMessage)
	err := c.graphql.DoWithContext(ctx, query, variables, &data)
	var graphQLErr *api.GraphQLError
	if err != nil && !errors.As(err, &graphQLErr) {
		markPreflightChunkError(chunk, results, err)
		return
	}
	parsePreflightData(data, metadata, results)
	if graphQLErr != nil {
		applyGraphQLErrors(graphQLErr, metadata, results)
	}
}

func buildPreflightQuery(chunk preflightChunk) (string, map[string]interface{}, graphQLChunkMetadata) {
	definitions := make([]string, 0)
	fields := make([]string, 0, len(chunk.pieces))
	variables := make(map[string]interface{})
	metadata := graphQLChunkMetadata{
		repositories: make(map[string]preflightPiece),
		refs:         make(map[string]map[string]string),
	}
	selection := graphQLTargetSelection(annotatedTagPeelLimit)
	for repositoryIndex, piece := range chunk.pieces {
		repositoryAlias := fmt.Sprintf("repo%d", repositoryIndex)
		ownerVariable := fmt.Sprintf("owner%d", repositoryIndex)
		nameVariable := fmt.Sprintf("name%d", repositoryIndex)
		definitions = append(definitions, "$"+ownerVariable+":String!", "$"+nameVariable+":String!")
		variables[ownerVariable] = piece.repository.Owner
		variables[nameVariable] = piece.repository.Name
		repositoryFields := make([]string, 0, len(piece.refs)+1)
		if piece.readPermission {
			repositoryFields = append(repositoryFields, "viewerPermission")
		}
		metadata.repositories[repositoryAlias] = piece
		metadata.refs[repositoryAlias] = make(map[string]string, len(piece.refs))
		for refIndex, ref := range piece.refs {
			refAlias := fmt.Sprintf("ref%d", refIndex)
			refVariable := fmt.Sprintf("ref%d_%d", repositoryIndex, refIndex)
			definitions = append(definitions, "$"+refVariable+":String!")
			variables[refVariable] = ref
			repositoryFields = append(repositoryFields,
				fmt.Sprintf("%s: ref(qualifiedName:$%s) { target { %s } }", refAlias, refVariable, selection),
			)
			metadata.refs[repositoryAlias][refAlias] = ref
		}
		fields = append(fields, fmt.Sprintf(
			"%s: repository(owner:$%s,name:$%s) { %s }",
			repositoryAlias, ownerVariable, nameVariable, strings.Join(repositoryFields, " "),
		))
	}
	query := "query StatusPreflight(" + strings.Join(definitions, ",") + ") { " + strings.Join(fields, " ") + " }"
	return query, variables, metadata
}

func graphQLTargetSelection(depth int) string {
	selection := "__typename oid"
	if depth > 0 {
		selection += " ... on Tag { target { " + graphQLTargetSelection(depth-1) + " } }"
	}
	return selection
}

func parsePreflightData(
	data map[string]json.RawMessage,
	metadata graphQLChunkMetadata,
	results map[string]status.PreflightResult,
) {
	for repositoryAlias, piece := range metadata.repositories {
		key := preflightRepositoryKey(piece.repository)
		result := results[key]
		raw, ok := data[repositoryAlias]
		if !ok || string(raw) == "null" {
			markPreflightPieceError(piece, &result, fmt.Errorf("repository unavailable"))
			results[key] = result
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			markPreflightPieceError(piece, &result, err)
			results[key] = result
			continue
		}
		if piece.readPermission {
			result.PermissionChecked = true
			result.CanPush, result.PermissionErr = parseViewerPermission(fields["viewerPermission"])
		}
		for refAlias, ref := range metadata.refs[repositoryAlias] {
			var response graphQLRef
			refRaw, ok := fields[refAlias]
			if !ok || string(refRaw) == "null" {
				result.Refs[ref] = status.PreflightRefResult{Err: fmt.Errorf("source ref unavailable")}
				continue
			}
			if err := json.Unmarshal(refRaw, &response); err != nil {
				result.Refs[ref] = status.PreflightRefResult{Err: err}
				continue
			}
			resolved, err := resolveGraphQLTarget(ref, response.Target)
			result.Refs[ref] = status.PreflightRefResult{Resolved: resolved, Err: err}
		}
		results[key] = result
	}
}

func parseViewerPermission(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, fmt.Errorf("viewerPermission is unavailable")
	}
	var permission string
	if err := json.Unmarshal(raw, &permission); err != nil {
		return false, err
	}
	switch permission {
	case "ADMIN", "MAINTAIN", "WRITE":
		return true, nil
	case "READ", "TRIAGE":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported viewerPermission %q", permission)
	}
}

func resolveGraphQLTarget(ref string, target graphQLTarget) (source.ResolvedRef, error) {
	resolved := source.ResolvedRef{RefSHA: target.OID}
	current := &target
	for depth := 0; depth < annotatedTagPeelLimit; depth++ {
		if current == nil || current.OID == "" || current.TypeName == "" {
			return source.ResolvedRef{}, fmt.Errorf("source ref %s has an incomplete target", ref)
		}
		switch current.TypeName {
		case "Commit":
			resolved.CommitSHA = current.OID
			return resolved, nil
		case "Tag":
			current = current.Target
		default:
			return source.ResolvedRef{}, fmt.Errorf("source ref %s does not resolve to a commit", ref)
		}
	}
	return source.ResolvedRef{}, fmt.Errorf("source ref %s exceeds annotated tag peel limit", ref)
}

func applyGraphQLErrors(
	graphQLErr *api.GraphQLError,
	metadata graphQLChunkMetadata,
	results map[string]status.PreflightResult,
) {
	for _, item := range graphQLErr.Errors {
		err := errors.New(item.Message)
		if len(item.Path) == 0 {
			for _, piece := range metadata.repositories {
				key := preflightRepositoryKey(piece.repository)
				result := results[key]
				markPreflightPieceError(piece, &result, err)
				results[key] = result
			}
			continue
		}
		repositoryAlias, ok := item.Path[0].(string)
		if !ok {
			continue
		}
		piece, ok := metadata.repositories[repositoryAlias]
		if !ok {
			continue
		}
		key := preflightRepositoryKey(piece.repository)
		result := results[key]
		if len(item.Path) == 1 {
			markPreflightPieceError(piece, &result, err)
		} else if field, ok := item.Path[1].(string); ok {
			switch field {
			case "viewerPermission":
				result.PermissionChecked = true
				result.PermissionErr = err
			default:
				if ref, exists := metadata.refs[repositoryAlias][field]; exists {
					result.Refs[ref] = status.PreflightRefResult{Err: err}
				}
			}
		}
		results[key] = result
	}
}

func markPreflightChunkError(
	chunk preflightChunk,
	results map[string]status.PreflightResult,
	err error,
) {
	for _, piece := range chunk.pieces {
		key := preflightRepositoryKey(piece.repository)
		result := results[key]
		markPreflightPieceError(piece, &result, err)
		results[key] = result
	}
}

func markPreflightPieceError(piece preflightPiece, result *status.PreflightResult, err error) {
	for _, ref := range piece.refs {
		result.Refs[ref] = status.PreflightRefResult{Err: err}
	}
	if piece.readPermission {
		result.PermissionChecked = true
		result.PermissionErr = err
	}
}

func preflightRepositoryKey(repository source.Repository) string {
	return repository.Owner + "/" + repository.Name
}
