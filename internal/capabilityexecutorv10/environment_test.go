package capabilityexecutorv10

import (
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/promotiontoolchain"
)

func TestPromotionEnvironmentHashExcludesHostPaths(t *testing.T) {
	first := testPromotionEnvironment()
	second := first
	second.KiCadCLI = "/different/kicad-cli"
	second.SymbolsRoot = "/different/symbols"
	second.FootprintsRoot = "/different/footprints"
	firstHash, err := PromotionEnvironmentHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := PromotionEnvironmentHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("host paths changed promotion environment hash: %s != %s", firstHash, secondHash)
	}
}

func TestRegularFileSHA256RejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "tool")
	if err := os.WriteFile(target, []byte("tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	hash, err := RegularFileSHA256(target)
	if err != nil || hash != testRawDigest([]byte("tool")) {
		t.Fatalf("regular tool hash = %q, %v", hash, err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := RegularFileSHA256(link); err == nil {
		t.Fatal("symlinked tool was accepted")
	}
}

func testPromotionEnvironment() promotiontoolchain.Evidence {
	return promotiontoolchain.Evidence{
		Schema: "kicadai.promotion-toolchain-evidence.v1", Version: 1, LockSHA256: testDigest("lock"),
		OS: "darwin", Arch: "arm64", KiCadVersion: "10.0.3", KiCadCLI: "/path/kicad-cli",
		SymbolsRoot: "/path/symbols", FootprintsRoot: "/path/footprints",
		SymbolTableSHA256: testDigest("sym-table"), FootprintTableSHA256: testDigest("fp-table"),
		SymbolsIdentity:    promotiontoolchain.LibraryIdentity{SHA256: testDigest("symbols"), FileCount: 1, ByteCount: 10},
		FootprintsIdentity: promotiontoolchain.LibraryIdentity{SHA256: testDigest("footprints"), FileCount: 1, ByteCount: 20},
		Resolution:         "locked",
	}
}
