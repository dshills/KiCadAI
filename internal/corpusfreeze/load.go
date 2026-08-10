package corpusfreeze

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	ots "kicadai/internal/opentopologysynthesis"
)

const maxAuthorshipBytes = 1 << 20

type Packet struct {
	Assignments map[string][]byte
	Binding     Binding
}

func LoadPacket(root string, policy Policy) (Packet, error) {
	root = filepath.Clean(root)
	if err := requireRealDirectory(root); err != nil {
		return Packet{}, fmt.Errorf("packet root: %w", err)
	}
	packetSetData, err := readRegularFileUnder(root, "PACKET_SET.sha256", maxAuthorshipBytes)
	if err != nil {
		return Packet{}, fmt.Errorf("read packet set: %w", err)
	}
	packetSetSHA256 := hashBytes(packetSetData)
	if policy.PacketSetSHA256 != "" && packetSetSHA256 != policy.PacketSetSHA256 {
		return Packet{}, fmt.Errorf("packet set does not match frozen policy")
	}
	verifiedDigests := map[string]string{}
	packetSet, err := verifyChecksumManifestCached(root, "PACKET_SET.sha256", verifiedDigests)
	if err != nil {
		return Packet{}, err
	}
	result := Packet{
		Assignments: map[string][]byte{},
		Binding: Binding{
			PacketSetSHA256:       packetSetSHA256,
			ContractBindingSHA256: packetSet["CONTRACT_BINDING.json"],
			AuthorPacketSHA256:    map[string]string{},
			AssignmentSHA256:      map[string]string{},
		},
	}
	for _, author := range policy.AuthorSlots {
		manifestName, err := authorPacketManifestName(author)
		if err != nil {
			return Packet{}, err
		}
		assignmentName := "assignments/" + author + ".json"
		if packetSet[manifestName] == "" || packetSet[assignmentName] == "" || packetSet["CONTRACT_BINDING.json"] == "" {
			return Packet{}, fmt.Errorf("packet set omits binding inputs for %s", author)
		}
		authorSet, err := verifyChecksumManifestCached(root, manifestName, verifiedDigests)
		if err != nil {
			return Packet{}, err
		}
		if authorSet[assignmentName] == "" || authorSet["CONTRACT_BINDING.json"] == "" {
			return Packet{}, fmt.Errorf("%s omits assignment or contract binding", manifestName)
		}
		assignmentBytes, err := readRegularFileUnder(root, assignmentName, maxAuthorshipBytes)
		if err != nil {
			return Packet{}, fmt.Errorf("read %s: %w", assignmentName, err)
		}
		assignment, err := DecodeAssignmentStrict(assignmentBytes)
		if err != nil {
			return Packet{}, fmt.Errorf("%s: %w", author, err)
		}
		if assignment.Schema != policy.AssignmentSchema || assignment.Version != policy.Version || assignment.AuthorSlot != author {
			return Packet{}, fmt.Errorf("%s assignment header is invalid", author)
		}
		result.Assignments[author] = assignmentBytes
		result.Binding.AuthorPacketSHA256[author] = packetSet[manifestName]
		result.Binding.AssignmentSHA256[author] = packetSet[assignmentName]
	}
	return result, nil
}

func LoadBundle(root string, assignmentData []byte) (Bundle, error) {
	root = filepath.Clean(root)
	if err := requireRealDirectory(root); err != nil {
		return Bundle{}, fmt.Errorf("bundle root: %w", err)
	}
	assignment, err := DecodeAssignmentStrict(assignmentData)
	if err != nil {
		return Bundle{}, err
	}
	wantFiles := map[string]bool{"AUTHORSHIP.json": true}
	wantDirectories := map[string]bool{".": true}
	for _, entry := range assignment.Entries {
		if !validRelativePath(entry.RequirementFile) {
			return Bundle{}, fmt.Errorf("assignment contains unsafe requirement path")
		}
		wantFiles[entry.RequirementFile] = true
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(entry.RequirementFile))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			wantDirectories[directory] = true
		}
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if !validRelativePath(relative) {
			return fmt.Errorf("bundle contains unsafe path")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle contains symlink %s", relative)
		}
		if entry.IsDir() {
			if !wantDirectories[relative] {
				return fmt.Errorf("bundle contains unexpected directory %s", relative)
			}
			return nil
		}
		if !entry.Type().IsRegular() || !wantFiles[relative] {
			return fmt.Errorf("bundle contains unexpected file %s", relative)
		}
		seen[relative] = true
		return nil
	})
	if err != nil {
		return Bundle{}, err
	}
	if len(seen) != len(wantFiles) {
		return Bundle{}, fmt.Errorf("bundle file count = %d, want %d", len(seen), len(wantFiles))
	}
	authorship, err := readRegularFileUnder(root, "AUTHORSHIP.json", maxAuthorshipBytes)
	if err != nil {
		return Bundle{}, fmt.Errorf("read authorship: %w", err)
	}
	bundle := Bundle{AuthorshipJSON: authorship, Requirements: map[string][]byte{}}
	for _, entry := range assignment.Entries {
		data, err := readRegularFileUnder(root, entry.RequirementFile, ots.MaxRequirementBytes)
		if err != nil {
			return Bundle{}, fmt.Errorf("read %s: %w", entry.RequirementFile, err)
		}
		bundle.Requirements[entry.RequirementFile] = data
	}
	return bundle, nil
}

func verifyChecksumManifest(root, name string) (map[string]string, error) {
	return verifyChecksumManifestCached(root, name, map[string]string{})
}

func verifyChecksumManifestCached(root, name string, verifiedDigests map[string]string) (map[string]string, error) {
	data, err := readRegularFileUnder(root, name, maxAuthorshipBytes)
	if err != nil {
		return nil, fmt.Errorf("read checksum manifest %s: %w", name, err)
	}
	result := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		// Author packet manifests are frozen in one canonical text form:
		// lowercase SHA-256, exactly two ASCII spaces, then a canonical path.
		// Binary-mode markers and flexible whitespace are intentionally rejected.
		if len(line) <= 66 || line[64:66] != "  " {
			return nil, fmt.Errorf("checksum manifest %s contains malformed entry", name)
		}
		digest, relative := line[:64], line[66:]
		if !validSHA256(digest) || !validRelativePath(relative) || result[relative] != "" {
			return nil, fmt.Errorf("checksum manifest %s contains invalid path or hash", name)
		}
		actual, ok := verifiedDigests[relative]
		if !ok {
			actual, err = hashRegularFileUnder(root, relative, maxAuthorshipBytes)
			if err != nil {
				return nil, fmt.Errorf("verify %s entry %s: %w", name, relative, err)
			}
			verifiedDigests[relative] = actual
		}
		if actual != digest {
			return nil, fmt.Errorf("checksum manifest %s hash mismatch for %s", name, relative)
		}
		result[relative] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan checksum manifest %s: %w", name, err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("checksum manifest %s is empty", name)
	}
	return result, nil
}

func readRegularFileUnder(root, relative string, maximum int64) ([]byte, error) {
	if !validRelativePath(filepath.ToSlash(relative)) {
		return nil, fmt.Errorf("unsafe relative path")
	}
	file, err := openRegularFileUnder(root, filepath.ToSlash(relative))
	if err != nil {
		return nil, err
	}
	return readBoundedAndClose(file, maximum)
}

func hashRegularFileUnder(root, relative string, maximum int64) (string, error) {
	if !validRelativePath(filepath.ToSlash(relative)) {
		return "", fmt.Errorf("unsafe relative path")
	}
	file, err := openRegularFileUnder(root, filepath.ToSlash(relative))
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(hasher, io.LimitReader(file, maximum+1))
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if written > maximum {
		return "", fmt.Errorf("file exceeds %d bytes", maximum)
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func readRegularFile(path string, maximum int64) ([]byte, error) {
	file, err := openVerifiedRegular(path)
	if err != nil {
		return nil, err
	}
	return readBoundedAndClose(file, maximum)
}

func readBoundedAndClose(file *os.File, maximum int64) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(file, maximum+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes", maximum)
	}
	return data, nil
}

func openVerifiedRegular(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, statErr
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("file changed while opening")
	}
	return file, nil
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("not a real directory")
	}
	return nil
}

func authorPacketManifestName(author string) (string, error) {
	parts := strings.Split(author, "_")
	if len(parts) != 2 || parts[0] != "author" || parts[1] == "" {
		return "", fmt.Errorf("unsupported author slot %q", author)
	}
	return "AUTHOR_" + strings.ToUpper(parts[1]) + "_PACKET.sha256", nil
}
