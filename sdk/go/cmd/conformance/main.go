package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	rizzbase "github.com/pazti/Rizz-Base/sdk/go"
)

type fixture struct {
	Name             string                     `json:"name"`
	SetupFiles       map[string]any             `json:"setupFiles"`
	SetupDirectories map[string]directoryConfig `json:"setupDirectories"`
	Steps            []fixtureStep              `json:"steps"`
}

type directoryConfig struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode"`
}

type fixtureStep struct {
	ID        string         `json:"id"`
	Operation string         `json:"operation"`
	Args      map[string]any `json:"args"`
	Expect    struct {
		ErrorCode string         `json:"errorCode"`
		Fields    map[string]any `json:"fields"`
	} `json:"expect"`
	Capture    map[string]string `json:"capture"`
	SkipIfRoot bool              `json:"skipIfRoot"`
}

func main() {
	if os.Getenv("RIZZBASE_BINARY") == "" {
		fail("Set RIZZBASE_BINARY to a compatible engine executable.")
	}
	fixtures, err := fixturePaths(os.Args[1:])
	if err != nil {
		fail(err.Error())
	}
	results := make([]any, 0, len(fixtures))
	for _, path := range fixtures {
		result, err := runFixture(path)
		if err != nil {
			fail(err.Error())
		}
		results = append(results, result)
	}
	if err := json.NewEncoder(os.Stdout).Encode(results); err != nil {
		fail(err.Error())
	}
}

func fixturePaths(args []string) ([]string, error) {
	if len(args) > 0 {
		paths := make([]string, len(args))
		for i, path := range args {
			absolute, err := filepath.Abs(path)
			if err != nil {
				return nil, err
			}
			paths[i] = absolute
		}
		return paths, nil
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "conformance", "fixtures"))
	if err != nil {
		return nil, fmt.Errorf("read conformance fixtures: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		paths = append(paths, filepath.Join("..", "..", "conformance", "fixtures", entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func runFixture(path string) (result map[string]any, resultErr error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var definition fixture
	if err := json.Unmarshal(data, &definition); err != nil {
		return nil, fmt.Errorf("decode fixture %s: %w", path, err)
	}
	directory, err := os.MkdirTemp("", "rizzbase-conformance-")
	if err != nil {
		return nil, err
	}
	references := map[string]any{
		"$project":                filepath.Join(directory, "project.rizz"),
		"$missingProject":         filepath.Join(directory, "missing.rizz"),
		"$invalidProject":         filepath.Join(directory, "invalid.rizz"),
		"$restoredProject":        filepath.Join(directory, "restored.rizz"),
		"$exportedBackup":         filepath.Join(directory, "exported.json"),
		"$missingBinary":          filepath.Join(directory, "missing-engine"),
		"$incompatibleBinary":     os.Getenv("RIZZBASE_INCOMPATIBLE_BINARY"),
		"$protocolErrorBinary":    os.Getenv("RIZZBASE_PROTOCOL_ERROR_BINARY"),
		"$nonzeroBinary":          os.Getenv("RIZZBASE_NONZERO_BINARY"),
		"$missingExportDirectory": filepath.Join(directory, "missing-directory"),
	}
	defer func() {
		var cleanupErrors []error
		for _, definition := range definition.SetupDirectories {
			setupPath := filepath.Join(directory, definition.Path)
			if err := os.Chmod(setupPath, 0700); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
		if err := os.RemoveAll(directory); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
		resultErr = errors.Join(resultErr, errors.Join(cleanupErrors...))
	}()
	for key, contents := range definition.SetupFiles {
		extension := ".json"
		if strings.HasSuffix(key, "Project") {
			extension = ".rizz"
		}
		path := filepath.Join(directory, key[1:]+extension)
		references[key] = path
		var fileData []byte
		switch value := contents.(type) {
		case string:
			fileData = []byte(value)
		case map[string]any:
			if size, ok := value["brainrotBytes"].(float64); ok {
				fileData = []byte(strings.TrimSpace(strings.Repeat("rizz rizz ", int(size))))
			} else {
				fileData, err = json.Marshal(value)
				if err != nil {
					return nil, err
				}
				fileData = append(fileData, '\n')
			}
		default:
			fileData, err = json.Marshal(value)
			if err != nil {
				return nil, err
			}
			fileData = append(fileData, '\n')
		}
		if err := os.WriteFile(path, fileData, 0600); err != nil {
			return nil, err
		}
	}
	for key, definition := range definition.SetupDirectories {
		path := filepath.Join(directory, definition.Path)
		if err := os.MkdirAll(path, os.FileMode(definition.Mode)); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, os.FileMode(definition.Mode)); err != nil {
			return nil, err
		}
		references[key] = path
	}

	generatedIDs := []string{}
	normalizedSteps := make([]any, 0, len(definition.Steps))
	for _, step := range definition.Steps {
		if step.SkipIfRoot && isRoot() {
			normalizedSteps = append(normalizedSteps, map[string]any{
				"step": step.ID, "result": map[string]any{"skipped": "requires a non-root user"},
			})
			continue
		}
		expanded, err := replaceReferences(step.Args, references)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", definition.Name, step.ID, err)
		}
		args, ok := expanded.(map[string]any)
		if !ok {
			args = map[string]any{}
		}
		projectPath := stringArg(args, "path", references["$project"].(string))
		passphrase := stringArg(args, "passphrase", "conformance-passphrase")
		binary := stringArg(args, "binaryPath", os.Getenv("RIZZBASE_BINARY"))
		options := []rizzbase.Option{rizzbase.WithBinaryPath(binary)}
		if timeout, ok := args["timeoutMs"].(float64); ok {
			options = append(options, rizzbase.WithTimeout(time.Duration(timeout)*time.Millisecond))
		}
		db, err := rizzbase.New(projectPath, passphrase, options...)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", definition.Name, step.ID, err)
		}

		result, operationErr := runOperation(step.Operation, args, db)
		if step.Expect.ErrorCode != "" {
			var sdkErr *rizzbase.Error
			if !errors.As(operationErr, &sdkErr) || string(sdkErr.Code) != step.Expect.ErrorCode {
				return nil, fmt.Errorf("%s/%s: expected %s, got %v", definition.Name, step.ID, step.Expect.ErrorCode, operationErr)
			}
		} else {
			if operationErr != nil {
				return nil, fmt.Errorf("%s/%s failed: %w", definition.Name, step.ID, operationErr)
			}
			for field, expected := range step.Expect.Fields {
				actual, exists := getPath(result, field)
				if !exists || !equalJSON(actual, expected) {
					return nil, fmt.Errorf("%s/%s field %s: expected %v, got %v", definition.Name, step.ID, field, expected, actual)
				}
			}
		}
		for name, field := range step.Capture {
			value, exists := getPath(result, field)
			if !exists || value == nil {
				return nil, fmt.Errorf("%s/%s did not provide %s", definition.Name, step.ID, field)
			}
			references[name] = value
			if id, ok := value.(string); ok {
				generatedIDs = append(generatedIDs, id)
			}
		}

		var normalized map[string]any
		if operationErr != nil {
			var sdkErr *rizzbase.Error
			_ = errors.As(operationErr, &sdkErr)
			normalized = map[string]any{"ok": false, "errorCode": string(sdkErr.Code)}
		} else {
			normalized = map[string]any{"ok": true}
			for key, value := range result {
				normalized[key] = value
			}
			if step.Operation == "lobby.list" && normalized["lobbies"] == nil {
				normalized["lobbies"] = []any{}
			}
			if (step.Operation == "yap.list" || step.Operation == "yap.query" || step.Operation == "yap.bulkCreate") && normalized["yaps"] == nil {
				normalized["yaps"] = []any{}
			}
		}
		normalizedValue, err := normalize(normalized, generatedIDs, directory)
		if err != nil {
			return nil, err
		}
		normalizedSteps = append(normalizedSteps, map[string]any{"step": step.ID, "result": normalizedValue})
	}
	return map[string]any{
		"fixture": definition.Name, "steps": len(definition.Steps), "results": normalizedSteps,
	}, nil
}

func runOperation(operation string, args map[string]any, db *rizzbase.DB) (map[string]any, error) {
	ctx := context.Background()
	switch operation {
	case "create":
		project, err := db.Create(ctx, stringArg(args, "name", ""))
		return map[string]any{"project": project}, err
	case "open":
		project, err := db.Open(ctx)
		return map[string]any{"project": project}, err
	case "sync.initialize":
		_, err := db.InitializeSync(ctx)
		return map[string]any{}, err
	case "sync.inspect":
		project, err := db.Open(ctx)
		if err != nil {
			return nil, err
		}
		if project.Sync == nil {
			return map[string]any{"sync": map[string]int{}}, nil
		}
		return map[string]any{"sync": map[string]int{
			"membershipCount": len(project.Sync.MembershipHistory),
			"operationCount":  len(project.Sync.Operations),
			"conflictCount":   len(project.Sync.Conflicts),
		}}, nil
	case "project.import":
		project, err := db.ImportBackup(ctx, stringArg(args, "backupPath", ""))
		return map[string]any{"project": project}, err
	case "project.export", "sdk.export":
		err := db.Export(ctx, stringArg(args, "backupPath", ""))
		return map[string]any{}, err
	case "lobby.create":
		lobby, err := db.CreateLobby(ctx, stringArg(args, "name", ""))
		return map[string]any{"lobby": lobby}, err
	case "lobby.list":
		lobbies, err := db.ListLobbies(ctx)
		return map[string]any{"lobbies": lobbies}, err
	case "lobby.delete":
		return map[string]any{}, db.DeleteLobby(ctx, stringArg(args, "lobbyId", ""))
	case "event.append":
		event, err := db.AppendEvent(ctx, stringArg(args, "kind", ""), stringArg(args, "source", ""), args["data"])
		return map[string]any{"event": event}, err
	case "event.list":
		events, err := db.ListEvents(ctx)
		return map[string]any{"events": events}, err
	case "event.query":
		options, err := eventQueryOptions(args["eventQuery"])
		if err != nil {
			return nil, err
		}
		page, err := db.QueryEvents(ctx, options)
		if err != nil {
			return nil, err
		}
		return map[string]any{"events": page.Events, "eventPage": page}, nil
	case "yap.create":
		yap, err := db.CreateYap(ctx, stringArg(args, "lobbyId", ""), stringArg(args, "yapId", ""), args["data"])
		return map[string]any{"yap": yap}, err
	case "yap.bulkCreate":
		inputs, err := yapInputs(args["yaps"])
		if err != nil {
			return nil, err
		}
		yaps, err := db.CreateYaps(ctx, stringArg(args, "lobbyId", ""), inputs)
		return map[string]any{"yaps": yaps}, err
	case "yap.get":
		data, err := db.GetYap(ctx, stringArg(args, "lobbyId", ""), stringArg(args, "yapId", ""))
		var value any
		if err == nil {
			err = json.Unmarshal(data, &value)
		}
		return map[string]any{"data": value}, err
	case "yap.list":
		yaps, err := db.ListYaps(ctx, stringArg(args, "lobbyId", ""))
		return map[string]any{"yaps": yaps}, err
	case "yap.query", "yap.queryPage":
		query, err := queryOptions(args["query"])
		if err != nil {
			return nil, err
		}
		if operation == "yap.query" {
			yaps, err := db.QueryYaps(ctx, stringArg(args, "lobbyId", ""), query)
			return map[string]any{"yaps": yaps}, err
		}
		page, err := db.QueryYapPage(ctx, stringArg(args, "lobbyId", ""), query)
		return map[string]any{"page": page}, err
	case "yap.update":
		yap, err := db.UpdateYap(ctx, stringArg(args, "lobbyId", ""), stringArg(args, "yapId", ""), args["data"])
		return map[string]any{"yap": yap}, err
	case "yap.delete":
		return map[string]any{}, db.DeleteYap(ctx, stringArg(args, "lobbyId", ""), stringArg(args, "yapId", ""))
	default:
		return nil, fmt.Errorf("unsupported fixture operation: %s", operation)
	}
}

func eventQueryOptions(value any) (rizzbase.EventQueryOptions, error) {
	if value == nil {
		return rizzbase.EventQueryOptions{}, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return rizzbase.EventQueryOptions{}, err
	}
	var options rizzbase.EventQueryOptions
	if err := json.Unmarshal(data, &options); err != nil {
		return rizzbase.EventQueryOptions{}, err
	}
	return options, nil
}

func yapInputs(value any) ([]rizzbase.YapInput, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("fixture yaps must be an array")
	}
	yaps := make([]rizzbase.YapInput, len(items))
	for i, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("fixture Yap must be an object")
		}
		yaps[i] = rizzbase.YapInput{ID: stringArg(fields, "id", ""), Data: fields["data"]}
	}
	return yaps, nil
}

func queryOptions(value any) (rizzbase.QueryOptions, error) {
	if value == nil {
		return rizzbase.QueryOptions{}, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return rizzbase.QueryOptions{}, err
	}
	var options rizzbase.QueryOptions
	if err := json.Unmarshal(data, &options); err != nil {
		return rizzbase.QueryOptions{}, err
	}
	return options, nil
}

func replaceReferences(value any, references map[string]any) (any, error) {
	switch item := value.(type) {
	case map[string]any:
		if repeat, ok := item["$repeat"].(map[string]any); ok {
			text, textOK := repeat["value"].(string)
			count, countOK := repeat["count"].(float64)
			if textOK && countOK && count >= 0 && count <= 1_000_000 {
				return strings.Repeat(text, int(count)), nil
			}
		}
		expanded := make(map[string]any, len(item))
		for key, nested := range item {
			result, err := replaceReferences(nested, references)
			if err != nil {
				return nil, err
			}
			expanded[key] = result
		}
		return expanded, nil
	case []any:
		expanded := make([]any, len(item))
		for i, nested := range item {
			result, err := replaceReferences(nested, references)
			if err != nil {
				return nil, err
			}
			expanded[i] = result
		}
		return expanded, nil
	case string:
		if value, ok := references[item]; ok {
			return value, nil
		}
		var replacementErr error
		expanded := referencePattern.ReplaceAllStringFunc(item, func(match string) string {
			if replacementErr != nil {
				return match
			}
			name := match[2 : len(match)-1]
			key := name
			if _, ok := references[key]; !ok {
				key = "$" + name
			}
			replacement, ok := references[key]
			if !ok {
				replacementErr = fmt.Errorf("unknown fixture reference: %s", name)
				return match
			}
			return fmt.Sprint(replacement)
		})
		return expanded, replacementErr
	default:
		return value, nil
	}
}

var referencePattern = regexpMustCompile(`\$\{[^}]+\}`)

func regexpMustCompile(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

func getPath(value any, path string) (any, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, false
	}
	current := decoded
	for _, segment := range strings.Split(path, ".") {
		switch item := current.(type) {
		case map[string]any:
			var exists bool
			current, exists = item[segment]
			if !exists {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if segment == "length" {
				current = len(item)
			} else if err != nil || index < 0 || index >= len(item) {
				return nil, false
			} else {
				current = item[index]
			}
		default:
			return nil, false
		}
	}
	return current, true
}

func equalJSON(left, right any) bool {
	leftData, leftErr := json.Marshal(left)
	rightData, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return reflect.DeepEqual(left, right)
	}
	var normalizedLeft, normalizedRight any
	_ = json.Unmarshal(leftData, &normalizedLeft)
	_ = json.Unmarshal(rightData, &normalizedRight)
	return reflect.DeepEqual(normalizedLeft, normalizedRight)
}

func normalize(value any, generatedIDs []string, directory string) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return normalizeValue(decoded, generatedIDs, directory, ""), nil
}

func normalizeValue(value any, generatedIDs []string, directory, key string) any {
	if key == "createdAt" {
		return "<timestamp>"
	}
	if key == "projectId" {
		if _, ok := value.(string); ok {
			return "<generated-id>"
		}
	}
	if key == "nextCursor" {
		if text, ok := value.(string); ok && text != "" {
			return "<cursor>"
		}
	}
	switch item := value.(type) {
	case string:
		if contains(generatedIDs, item) {
			return "<generated-id>"
		}
		return strings.ReplaceAll(item, directory, "<temporary-directory>")
	case []any:
		result := make([]any, len(item))
		for i, nested := range item {
			result[i] = normalizeValue(nested, generatedIDs, directory, "")
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(item))
		for name, nested := range item {
			normalizedKey := name
			if contains(generatedIDs, name) {
				normalizedKey = "<generated-id>"
			}
			result[normalizedKey] = normalizeValue(nested, generatedIDs, directory, name)
		}
		return result
	default:
		return value
	}
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func stringArg(args map[string]any, key, fallback string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return fallback
}

func isRoot() bool {
	if runtime.GOOS != "windows" {
		return os.Geteuid() == 0
	}
	return false
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
