package promotionrunner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/promotiontoolchain"
)

const ComparisonSchema = "kicadai.promotion-comparison.v1"
const maxJSONBytes int64 = 256 << 20
const maxKiCadTextBytes int64 = 64 << 20
const maxManifestBytes int64 = 16 << 20
const maxJSONObjectBytes = 64 << 20
const maxJSONDepth = 100

var temporaryCheckPattern = regexp.MustCompile(`(?:[A-Za-z]:)?(?:[\\/][A-Za-z0-9._ -]+)*[\\/]kicadai-check-(?:drc|erc)-[0-9]+`)

type NormalizedFile struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Difference struct {
	Path   string `json:"path"`
	Run1   string `json:"run_1,omitempty"`
	Run2   string `json:"run_2,omitempty"`
	Reason string `json:"reason"`
}

type Comparison struct {
	Schema      string           `json:"schema"`
	Scenario    string           `json:"scenario"`
	Status      string           `json:"status"`
	Run1SHA256  string           `json:"run_1_sha256"`
	Run2SHA256  string           `json:"run_2_sha256"`
	Files       []NormalizedFile `json:"files,omitempty"`
	Differences []Difference     `json:"differences"`
}

type normalizationContext struct {
	projectRoot    string
	runRoot        string
	repositoryRoot string
	toolchain      promotiontoolchain.Evidence
	replacer       *strings.Replacer
}

type normalizationReplacement struct {
	from string
	to   string
}

func newNormalizationContext(projectRoot, repositoryRoot string, toolchain promotiontoolchain.Evidence) normalizationContext {
	context := normalizationContext{
		projectRoot: filepath.Clean(projectRoot), runRoot: filepath.Dir(filepath.Clean(projectRoot)),
		repositoryRoot: filepath.Clean(repositoryRoot), toolchain: toolchain,
	}
	replacements := []normalizationReplacement{
		{context.projectRoot, "${PROJECT}"},
		{context.runRoot, "${RUN}"},
		{context.repositoryRoot, "${REPOSITORY}"},
		{context.toolchain.KiCadCLI, "${KICAD_CLI}"},
		{context.toolchain.SymbolsRoot, "${SYMBOLS_ROOT}"},
		{context.toolchain.FootprintsRoot, "${FOOTPRINTS_ROOT}"},
		{context.toolchain.SymbolTable, "${SYMBOL_TABLE}"},
		{context.toolchain.FootprintTable, "${FOOTPRINT_TABLE}"},
	}
	slices.SortFunc(replacements, func(left, right normalizationReplacement) int {
		return len(right.from) - len(left.from)
	})
	var replacementPairs []string
	for _, replacement := range replacements {
		if replacement.from == "" || replacement.from == "." {
			continue
		}
		slashed := filepath.ToSlash(replacement.from)
		replacementPairs = append(replacementPairs, slashed, replacement.to)
		if slashed != replacement.from {
			replacementPairs = append(replacementPairs, replacement.from, replacement.to)
		}
	}
	context.replacer = strings.NewReplacer(replacementPairs...)
	return context
}

func CompareProjects(scenario, firstProject, secondProject, repositoryRoot string, toolchain promotiontoolchain.Evidence) (Comparison, error) {
	comparison := Comparison{
		Schema: ComparisonSchema, Scenario: scenario, Status: "failed", Differences: []Difference{},
	}
	first, err := normalizedInventory(firstProject, repositoryRoot, toolchain)
	if err != nil {
		return comparison, fmt.Errorf("normalize run 1: %w", err)
	}
	second, err := normalizedInventory(secondProject, repositoryRoot, toolchain)
	if err != nil {
		return comparison, fmt.Errorf("normalize run 2: %w", err)
	}
	firstHash := inventorySHA256(first)
	secondHash := inventorySHA256(second)
	comparison.Status = "pass"
	comparison.Run1SHA256 = firstHash
	comparison.Run2SHA256 = secondHash
	comparison.Differences = compareInventories(first, second)
	if comparison.Differences == nil {
		comparison.Differences = []Difference{}
	}
	comparison.Files = first
	if len(comparison.Differences) != 0 {
		comparison.Status = "failed"
		return comparison, fmt.Errorf("normalized project inventories differ")
	}
	return comparison, nil
}

func normalizedInventory(projectRoot, repositoryRoot string, toolchain promotiontoolchain.Evidence) ([]NormalizedFile, error) {
	context := newNormalizationContext(projectRoot, repositoryRoot, toolchain)
	var inventory []NormalizedFile
	hashes := make(map[string]string)
	err := filepath.WalkDir(projectRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("project contains symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("project contains non-regular file %q", path)
		}
		relative, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == ".kicadai/manifest.json" {
			return nil
		}
		normalizedFile, err := inventoryFile(path, relative, context)
		if err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		hashes[relative] = normalizedFile.SHA256
		inventory = append(inventory, normalizedFile)
		return nil
	})
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(projectRoot, ".kicadai", "manifest.json")
	if manifestRaw, err := readBoundedFile(manifestPath, maxManifestBytes); err == nil {
		manifest, err := normalizeManifest(manifestRaw, context, hashes)
		if err != nil {
			return nil, fmt.Errorf(".kicadai/manifest.json: %w", err)
		}
		inventory = append(inventory, NormalizedFile{
			Path: ".kicadai/manifest.json", Kind: "json",
			SHA256: hashBytes(manifest), Bytes: int64(len(manifest)),
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	slices.SortFunc(inventory, func(left, right NormalizedFile) int { return strings.Compare(left.Path, right.Path) })
	return inventory, nil
}

func inventoryFile(path, relative string, context normalizationContext) (NormalizedFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return NormalizedFile{}, err
	}
	defer file.Close()
	if !strings.HasSuffix(relative, ".json") && !isKiCadFile(relative) {
		hash := sha256.New()
		size, err := io.Copy(hash, file)
		if err != nil {
			return NormalizedFile{}, err
		}
		return NormalizedFile{
			Path: relative, Kind: "bytes", SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: size,
		}, nil
	}
	info, err := file.Stat()
	if err != nil {
		return NormalizedFile{}, err
	}
	limit := maxKiCadTextBytes
	if strings.HasSuffix(relative, ".json") {
		limit = maxJSONBytes
	}
	if info.Size() > limit {
		return NormalizedFile{}, fmt.Errorf(
			"parse-required file is %d bytes; normalization limit is %d", info.Size(), limit,
		)
	}
	if strings.HasSuffix(relative, ".json") {
		hash := sha256.New()
		counter := &countingWriter{writer: hash}
		limited := &io.LimitedReader{R: file, N: limit + 1}
		if err := normalizeJSONReader(limited, counter, context); err != nil {
			return NormalizedFile{}, err
		}
		if limited.N == 0 {
			return NormalizedFile{}, fmt.Errorf("parse-required file grew beyond normalization limit %d", limit)
		}
		return NormalizedFile{
			Path: relative, Kind: "json", SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: counter.bytes,
		}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return NormalizedFile{}, err
	}
	if int64(len(raw)) > limit {
		return NormalizedFile{}, fmt.Errorf("parse-required file grew beyond normalization limit %d", limit)
	}
	normalized, err := normalizeKiCadFile(relative, raw)
	if err != nil {
		return NormalizedFile{}, err
	}
	return NormalizedFile{
		Path: relative, Kind: "kicad", SHA256: hashBytes(normalized), Bytes: int64(len(normalized)),
	}, nil
}

type countingWriter struct {
	writer io.Writer
	bytes  int64
}

func (writer *countingWriter) Write(value []byte) (int, error) {
	written, err := writer.writer.Write(value)
	writer.bytes += int64(written)
	return written, err
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file is %d bytes; limit is %d", info.Size(), limit)
	}
	value, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(value)) > limit {
		return nil, fmt.Errorf("file grew beyond limit %d", limit)
	}
	return value, nil
}

func normalizeKiCadFile(relative string, raw []byte) ([]byte, error) {
	if !isKiCadFile(relative) {
		return nil, fmt.Errorf("%q is not a KiCad file", relative)
	}
	// The strict round-trip normalizer is the sole KiCad field normalizer.
	// Promotion bundles are platform-specific; masking KiCad fields here could
	// hide writer regressions that the deterministic replay is meant to expose.
	return []byte(roundtrip.NormalizeBytes(raw)), nil
}

func isKiCadFile(path string) bool {
	for _, suffix := range []string{
		".kicad_pcb", ".kicad_sch", ".kicad_pro", ".kicad_mod", ".kicad_sym", ".kicad_dru", ".kicad_wks",
	} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return filepath.Base(path) == "sym-lib-table" || filepath.Base(path) == "fp-lib-table"
}

func normalizeJSON(raw []byte, context normalizationContext) ([]byte, error) {
	var normalized bytes.Buffer
	if err := normalizeJSONReader(bytes.NewReader(raw), &normalized, context); err != nil {
		return nil, err
	}
	return normalized.Bytes(), nil
}

func normalizeJSONReader(reader io.Reader, writer io.Writer, context normalizationContext) error {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if err := writeNormalizedJSONValue(decoder, writer, "", context, 0); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNormalizedJSONValue(decoder *json.Decoder, writer io.Writer, key string, context normalizationContext, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds maximum depth %d", maxJSONDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			type objectField struct {
				key   string
				value []byte
			}
			seen := map[string]struct{}{}
			var fields []objectField
			objectBytes := 0
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				childKey, ok := keyToken.(string)
				if !ok {
					return errors.New("JSON object key is not a string")
				}
				if _, duplicate := seen[childKey]; duplicate {
					return fmt.Errorf("duplicate JSON object key %q", childKey)
				}
				seen[childKey] = struct{}{}
				var normalizedValue bytes.Buffer
				if err := writeNormalizedJSONValue(decoder, &normalizedValue, childKey, context, depth+1); err != nil {
					return err
				}
				objectBytes += len(childKey) + normalizedValue.Len()
				if objectBytes > maxJSONObjectBytes {
					return fmt.Errorf("normalized JSON object exceeds maximum size %d", maxJSONObjectBytes)
				}
				fields = append(fields, objectField{key: childKey, value: normalizedValue.Bytes()})
			}
			closeToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if closeToken != json.Delim('}') {
				return errors.New("unterminated JSON object")
			}
			slices.SortFunc(fields, func(left, right objectField) int {
				return strings.Compare(left.key, right.key)
			})
			if _, err := io.WriteString(writer, "{"); err != nil {
				return err
			}
			for index, field := range fields {
				if index != 0 {
					if _, err := io.WriteString(writer, ","); err != nil {
						return err
					}
				}
				encodedKey, err := json.Marshal(field.key)
				if err != nil {
					return err
				}
				if _, err := writer.Write(encodedKey); err != nil {
					return err
				}
				if _, err := io.WriteString(writer, ":"); err != nil {
					return err
				}
				if _, err := writer.Write(field.value); err != nil {
					return err
				}
			}
			_, err = io.WriteString(writer, "}")
			return err
		case '[':
			if _, err := io.WriteString(writer, "["); err != nil {
				return err
			}
			first := true
			for decoder.More() {
				if !first {
					if _, err := io.WriteString(writer, ","); err != nil {
						return err
					}
				}
				first = false
				if err := writeNormalizedJSONValue(decoder, writer, key, context, depth+1); err != nil {
					return err
				}
			}
			closeToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if closeToken != json.Delim(']') {
				return errors.New("unterminated JSON array")
			}
			_, err = io.WriteString(writer, "]")
			return err
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
	}
	switch typed := token.(type) {
	case string:
		if key == "generated_at" {
			typed = "${GENERATED_AT}"
		} else {
			typed = normalizeString(typed, context)
		}
		return writeJSONScalar(writer, typed)
	case json.Number:
		// Only numeric duration_ms telemetry is nondeterministic. A structured
		// value is semantic schema content and must remain comparison-visible.
		if key == "duration_ms" {
			typed = json.Number("0")
		}
		return writeJSONScalar(writer, typed)
	default:
		return writeJSONScalar(writer, token)
	}
}

func writeJSONScalar(writer io.Writer, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = writer.Write(encoded)
	return err
}

func decodeNormalizedJSON(raw []byte, context normalizationContext) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return normalizeJSONValue("", value, context, 0)
}

func normalizeJSONValue(key string, value any, context normalizationContext, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, fmt.Errorf("JSON nesting exceeds maximum depth %d", maxJSONDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			normalized, err := normalizeJSONValue(childKey, child, context, depth+1)
			if err != nil {
				return nil, err
			}
			typed[childKey] = normalized
		}
		return typed, nil
	case []any:
		for index, child := range typed {
			normalized, err := normalizeJSONValue(key, child, context, depth+1)
			if err != nil {
				return nil, err
			}
			typed[index] = normalized
		}
		return typed, nil
	case string:
		if key == "generated_at" {
			return "${GENERATED_AT}", nil
		}
		return normalizeString(typed, context), nil
	case json.Number:
		// Keep this policy aligned with the streaming normalizer above.
		if key == "duration_ms" {
			return json.Number("0"), nil
		}
		return typed, nil
	default:
		return value, nil
	}
}

func normalizeString(value string, context normalizationContext) string {
	normalized, pathChanged := maskKnownPaths(value, context)
	if pathChanged {
		normalized = strings.ReplaceAll(normalized, `\`, "/")
	}
	return normalized
}

func maskKnownPaths(value string, context normalizationContext) (string, bool) {
	original := value
	if context.replacer != nil {
		value = context.replacer.Replace(value)
	}
	normalized := temporaryCheckPattern.ReplaceAllStringFunc(value, func(string) string {
		return "${KICAD_CHECK}"
	})
	return normalized, normalized != original
}

func normalizeManifest(raw []byte, context normalizationContext, normalizedHashes map[string]string) ([]byte, error) {
	value, err := decodeNormalizedJSON(raw, context)
	if err != nil {
		return nil, err
	}
	manifest, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("manifest must be a JSON object")
	}
	if fileHashes, ok := manifest["file_hashes"].(map[string]any); ok {
		for path := range fileHashes {
			if hash, exists := normalizedHashes[path]; exists {
				fileHashes[path] = hash
			}
		}
	}
	if err := rewriteManifestHashReferences(manifest, normalizedHashes, 0); err != nil {
		return nil, err
	}
	return json.Marshal(manifest)
}

func rewriteManifestHashReferences(value any, normalizedHashes map[string]string, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("manifest nesting exceeds maximum depth %d", maxJSONDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		path, hasPath := typed["path"].(string)
		if _, hasHash := typed["sha256"]; hasPath && hasHash {
			if hash, exists := normalizedHashes[path]; exists {
				typed["sha256"] = hash
			}
		}
		for _, child := range typed {
			if err := rewriteManifestHashReferences(child, normalizedHashes, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rewriteManifestHashReferences(child, normalizedHashes, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func compareInventories(first, second []NormalizedFile) []Difference {
	firstByPath := make(map[string]NormalizedFile, len(first))
	secondByPath := make(map[string]NormalizedFile, len(second))
	for _, file := range first {
		firstByPath[file.Path] = file
	}
	for _, file := range second {
		secondByPath[file.Path] = file
	}
	paths := make([]string, 0, len(firstByPath)+len(secondByPath))
	for path := range firstByPath {
		paths = append(paths, path)
	}
	for path := range secondByPath {
		if _, exists := firstByPath[path]; !exists {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	var differences []Difference
	for _, path := range paths {
		left, leftOK := firstByPath[path]
		right, rightOK := secondByPath[path]
		switch {
		case !leftOK:
			differences = append(differences, Difference{Path: path, Run2: right.SHA256, Reason: "missing from run 1"})
		case !rightOK:
			differences = append(differences, Difference{Path: path, Run1: left.SHA256, Reason: "missing from run 2"})
		case left.Kind != right.Kind || left.SHA256 != right.SHA256 || left.Bytes != right.Bytes:
			differences = append(differences, Difference{Path: path, Run1: left.SHA256, Run2: right.SHA256, Reason: "normalized content differs"})
		}
	}
	return differences
}

func inventorySHA256(files []NormalizedFile) string {
	hash := sha256.New()
	for _, file := range files {
		_, _ = hash.Write([]byte(file.Path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.Kind))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.SHA256))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(file.Bytes, 10)))
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
