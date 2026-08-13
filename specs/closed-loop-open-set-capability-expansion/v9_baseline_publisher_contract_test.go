package closedloopopensetcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinepublicationv9"
)

func TestVersionNineBaselinePublisherIsFrozenAndPublicOnly(t *testing.T) {
	directory := v9BaselinePublisherContractDirectory(t)
	var freeze struct {
		Schema               string `json:"schema"`
		Version              int    `json:"version"`
		ImplementationCommit string `json:"implementation_commit"`
		FreezeParentCommit   string `json:"freeze_parent_commit"`
		PublisherManifest    string `json:"publisher_manifest"`
		PublisherSHA256      string `json:"publisher_manifest_sha256"`
		DiscoveryCount       int    `json:"discovery_count"`
		HeldOutAccessSurface bool   `json:"held_out_access_surface"`
		RealCorpusEvaluated  bool   `json:"real_corpus_evaluated"`
		ExternalKeyOpened    bool   `json:"external_key_opened"`
	}
	if err := json.Unmarshal(v9BaselinePublisherReadFile(t, filepath.Join(directory, "V9_BASELINE_PUBLISHER_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-baseline-publisher-freeze.v9" || freeze.Version != 9 ||
		freeze.ImplementationCommit != "5bd1c2cdc86be8d05a7a567426545942e8c4cb23" || freeze.FreezeParentCommit != "ccfe7d2e04eca6c318485d70a38f5d40255c8ff0" {
		t.Fatalf("invalid V9 baseline publisher freeze: %+v", freeze)
	}
	if freeze.PublisherManifest != "V9_BASELINE_PUBLISHER.sha256" || freeze.PublisherSHA256 != v9BaselinePublisherFileSHA256(t, filepath.Join(directory, freeze.PublisherManifest)) {
		t.Fatal("V9 baseline publisher manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || capabilitybaselinepublicationv9.ExpectedCases != 24 || freeze.HeldOutAccessSurface || freeze.RealCorpusEvaluated || freeze.ExternalKeyOpened {
		t.Fatal("V9 baseline publisher freeze crosses its public-only preparation boundary")
	}
	v9VerifyBaselinePublisherManifest(t, directory, freeze.PublisherManifest)
	assertV9BaselinePublisherImportsArePublicOnly(t, directory, freeze.PublisherManifest)
}

func assertV9BaselinePublisherImportsArePublicOnly(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v9BaselinePublisherReadFile(t, filepath.Join(directory, manifestName)))
	if strings.Contains(manifest, "\r") {
		t.Fatal("V9 baseline publisher manifest must use canonical LF line endings")
	}
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed V9 baseline publisher manifest line %q", line)
		}
		relative := fields[1]
		if !strings.HasSuffix(relative, ".go") {
			continue
		}
		if !strings.HasPrefix(relative, "../../internal/capabilitybaselinepublicationv9/") {
			t.Fatalf("V9 baseline publisher manifest contains out-of-package Go source %q", relative)
		}
		if strings.HasSuffix(relative, "_test.go") {
			continue
		}
		productionSources++
		source := v9BaselinePublisherReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(path, "kicadai/") && path != "kicadai/internal/atomicdir" && path != "kicadai/internal/capabilitybaselinev9" {
				t.Fatalf("V9 public baseline publisher %s imports forbidden package %q", relative, path)
			}
		}
	}
	if productionSources != 4 {
		t.Fatalf("V9 baseline publisher manifest names %d production Go sources, want 4", productionSources)
	}
}

func v9VerifyBaselinePublisherManifest(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v9BaselinePublisherReadFile(t, filepath.Join(directory, manifestName)))
	if strings.Contains(manifest, "\r") {
		t.Fatal("V9 baseline publisher manifest must use canonical LF line endings")
	}
	lines := strings.Split(strings.TrimSuffix(manifest, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("V9 baseline publisher manifest has %d entries, want 6", len(lines))
	}
	previous := ""
	for index, line := range lines {
		if len(line) < 67 || line[64:66] != "  " {
			t.Fatalf("V9 baseline publisher manifest entry %d is malformed", index)
		}
		digest, relative := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(digest) != digest || relative <= previous {
			t.Fatalf("V9 baseline publisher manifest entry %d is invalid or unordered", index)
		}
		artifactPath := v9BaselinePublisherArtifactPath(t, directory, relative)
		if got := v9BaselinePublisherFileSHA256(t, artifactPath); got != digest {
			t.Fatalf("V9 baseline publisher hash for %s = %s, want %s", relative, got, digest)
		}
		previous = relative
	}
}

func v9BaselinePublisherContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V9 baseline publisher contract directory")
	}
	return filepath.Dir(file)
}

func v9BaselinePublisherReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v9BaselinePublisherFileSHA256(t *testing.T, filePath string) string {
	t.Helper()
	pathInfo, err := os.Lstat(filePath)
	if err != nil || !pathInfo.Mode().IsRegular() || pathInfo.Size() < 0 || pathInfo.Size() > 32<<20 {
		t.Fatalf("V9 baseline publisher artifact is not a bounded regular file: %s", filePath)
	}
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil || !openInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openInfo) {
		t.Fatalf("V9 baseline publisher artifact changed while opening: %s", filePath)
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, (32<<20)+1))
	if err != nil {
		t.Fatal(err)
	}
	if written > 32<<20 {
		t.Fatalf("V9 baseline publisher artifact exceeds size limit: %s", filePath)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func v9BaselinePublisherArtifactPath(t *testing.T, directory, relative string) string {
	t.Helper()
	if relative == "" || path.IsAbs(relative) || filepath.IsAbs(relative) || strings.Contains(relative, `\`) || path.Clean(relative) != relative {
		t.Fatalf("invalid V9 baseline publisher manifest path %q", relative)
	}
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	artifact, err := filepath.Abs(filepath.Join(directory, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatal(err)
	}
	within, err := filepath.Rel(repository, artifact)
	if err != nil || within == ".." || filepath.IsAbs(within) || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		t.Fatalf("V9 baseline publisher manifest path escapes the repository: %q", relative)
	}
	return artifact
}
