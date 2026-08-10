package corpusfreeze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLoadPacketAndExactBundle(t *testing.T) {
	packetRoot, assignmentData, policy := writeTestPacket(t)
	packet, err := LoadPacket(packetRoot, policy)
	if err != nil {
		t.Fatal(err)
	}
	if string(packet.Assignments["author_1"]) != string(assignmentData) {
		t.Fatal("assignment bytes changed")
	}
	if !validSHA256(packet.Binding.PacketSetSHA256) || !validSHA256(packet.Binding.ContractBindingSHA256) || !validSHA256(packet.Binding.AuthorPacketSHA256["author_1"]) || !validSHA256(packet.Binding.AssignmentSHA256["author_1"]) {
		t.Fatalf("binding = %#v", packet.Binding)
	}

	bundleRoot := writeTestBundle(t)
	bundle, err := LoadBundle(bundleRoot, assignmentData)
	if err != nil {
		t.Fatal(err)
	}
	if string(bundle.AuthorshipJSON) != "{}\n" || string(bundle.Requirements["discovery/request_001.json"]) != "{}\n" {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestLoadPacketRejectsTamperingAndSymlinks(t *testing.T) {
	t.Run("frozen packet substitution", func(t *testing.T) {
		root, _, policy := writeTestPacket(t)
		policy.PacketSetSHA256 = strings.Repeat("f", 64)
		if _, err := LoadPacket(root, policy); err == nil || !strings.Contains(err.Error(), "does not match frozen policy") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		root, _, policy := writeTestPacket(t)
		mustWriteTestFile(t, filepath.Join(root, "CONTRACT_BINDING.json"), []byte("tampered\n"))
		if _, err := LoadPacket(root, policy); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("symlink input", func(t *testing.T) {
		root, _, policy := writeTestPacket(t)
		target := filepath.Join(root, "binding-target.json")
		mustWriteTestFile(t, target, []byte("{}\n"))
		if err := os.Remove(filepath.Join(root, "CONTRACT_BINDING.json")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "CONTRACT_BINDING.json")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := LoadPacket(root, policy); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("symlink parent", func(t *testing.T) {
		root, assignmentData, policy := writeTestPacket(t)
		assignments := filepath.Join(root, "assignments")
		if err := os.RemoveAll(assignments); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		mustWriteTestFile(t, filepath.Join(target, "author_1.json"), assignmentData)
		if err := os.Symlink(target, assignments); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := LoadPacket(root, policy); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestVerifyChecksumManifestRejectsNoncanonicalBinaryMarker(t *testing.T) {
	root := t.TempDir()
	contents := []byte("{}\n")
	mustWriteTestFile(t, filepath.Join(root, "input.json"), contents)
	manifest := []byte(fmt.Sprintf("%s *input.json\n", hashBytes(contents)))
	mustWriteTestFile(t, filepath.Join(root, "MANIFEST.sha256"), manifest)
	if _, err := verifyChecksumManifest(root, "MANIFEST.sha256"); err == nil || !strings.Contains(err.Error(), "malformed entry") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadBundleRejectsFilesystemAmbiguity(t *testing.T) {
	_, assignmentData, _ := writeTestPacket(t)
	for name, mutate := range map[string]func(*testing.T, string){
		"extra file": func(t *testing.T, root string) {
			mustWriteTestFile(t, filepath.Join(root, "extra.json"), []byte("{}\n"))
		},
		"missing file": func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "discovery", "request_001.json")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink file": func(t *testing.T, root string) {
			path := filepath.Join(root, "discovery", "request_001.json")
			target := filepath.Join(root, "target.json")
			mustWriteTestFile(t, target, []byte("{}\n"))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		},
		"extra directory": func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "unused"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeTestBundle(t)
			mutate(t, root)
			if _, err := LoadBundle(root, assignmentData); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func writeTestPacket(t *testing.T) (string, []byte, Policy) {
	t.Helper()
	root := t.TempDir()
	policy := V5Policy()
	policy.AuthorSlots = []string{"author_1"}
	policy.PacketSetSHA256 = ""
	assignment := Assignment{
		Schema: policy.AssignmentSchema, Version: policy.Version, AuthorSlot: "author_1",
		Entries: []AssignmentEntry{{
			ID: "case_001", Role: RoleDiscovery, Domain: "analog", SafetyImpact: "non_safety",
			SourceID: "source_001", RequirementFile: "discovery/request_001.json",
		}},
	}
	assignmentData, err := json.MarshalIndent(assignment, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assignmentData = append(assignmentData, '\n')
	contractData := []byte("{}\n")
	mustWriteTestFile(t, filepath.Join(root, "CONTRACT_BINDING.json"), contractData)
	mustWriteTestFile(t, filepath.Join(root, "assignments", "author_1.json"), assignmentData)
	authorEntries := map[string][]byte{
		"CONTRACT_BINDING.json":     contractData,
		"assignments/author_1.json": assignmentData,
	}
	authorManifest := testChecksumManifest(authorEntries)
	mustWriteTestFile(t, filepath.Join(root, "AUTHOR_1_PACKET.sha256"), authorManifest)
	packetEntries := map[string][]byte{
		"CONTRACT_BINDING.json":     contractData,
		"assignments/author_1.json": assignmentData,
		"AUTHOR_1_PACKET.sha256":    authorManifest,
	}
	mustWriteTestFile(t, filepath.Join(root, "PACKET_SET.sha256"), testChecksumManifest(packetEntries))
	return root, assignmentData, policy
}

func writeTestBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWriteTestFile(t, filepath.Join(root, "AUTHORSHIP.json"), []byte("{}\n"))
	mustWriteTestFile(t, filepath.Join(root, "discovery", "request_001.json"), []byte("{}\n"))
	return root
}

func testChecksumManifest(entries map[string][]byte) []byte {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		fmt.Fprintf(&builder, "%s  %s\n", hashBytes(entries[name]), name)
	}
	return []byte(builder.String())
}

func mustWriteTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
