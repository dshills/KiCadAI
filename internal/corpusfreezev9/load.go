package corpusfreezev9

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"kicadai/internal/corpusfreeze"
)

const maximumPacketFileBytes = 4 << 20

func LoadPacket(root string, policy Policy) (Packet, error) {
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Packet{}, fmt.Errorf("packet root is not a real directory")
	}
	packetSetData, err := readRegularFileUnder(root, "PACKET_SET.sha256")
	if err != nil {
		return Packet{}, fmt.Errorf("read packet set: %w", err)
	}
	packetSetHash := hashBytes(packetSetData)
	if packetSetHash != policy.PacketSetSHA256 {
		return Packet{}, fmt.Errorf("packet set does not match frozen V9 policy")
	}
	packetSet, err := verifyManifestData(root, "PACKET_SET.sha256", packetSetData)
	if err != nil {
		return Packet{}, err
	}
	result := Packet{Assignments: map[string][]byte{}, Binding: Binding{
		PacketSetSHA256: packetSetHash, ContractBindingSHA256: packetSet["CONTRACT_BINDING.json"],
		AuthorPacketSHA256: map[string]string{}, AssignmentSHA256: map[string]string{},
	}}
	for _, author := range policy.AuthorSlots {
		number := strings.TrimPrefix(author, "author_")
		manifest := "AUTHOR_" + number + "_PACKET.sha256"
		assignmentPath := "assignments/" + author + ".json"
		if packetSet[manifest] == "" || packetSet[assignmentPath] == "" || packetSet["CONTRACT_BINDING.json"] == "" {
			return Packet{}, fmt.Errorf("packet set omits binding inputs for %s", author)
		}
		authorSet, err := verifyManifest(root, manifest)
		if err != nil {
			return Packet{}, err
		}
		if authorSet[assignmentPath] == "" || authorSet["CONTRACT_BINDING.json"] == "" {
			return Packet{}, fmt.Errorf("%s omits assignment or contract binding", manifest)
		}
		data, err := readRegularFileUnder(root, assignmentPath)
		if err != nil {
			return Packet{}, err
		}
		assignment, err := decodeAssignment(data)
		if err != nil {
			return Packet{}, err
		}
		if assignment.Schema != policy.AssignmentSchema || assignment.Version != policy.Version || assignment.AuthorSlot != author {
			return Packet{}, fmt.Errorf("%s assignment header is invalid", author)
		}
		result.Assignments[author] = data
		result.Binding.AuthorPacketSHA256[author] = packetSet[manifest]
		result.Binding.AssignmentSHA256[author] = packetSet[assignmentPath]
	}
	return result, nil
}

func LoadBundle(root string, assignmentData []byte) (corpusfreeze.Bundle, error) {
	assignment, err := decodeAssignment(assignmentData)
	if err != nil {
		return corpusfreeze.Bundle{}, err
	}
	legacy := corpusfreeze.Assignment{Schema: assignment.Schema, Version: assignment.Version, AuthorSlot: assignment.AuthorSlot}
	for _, entry := range assignment.Entries {
		legacy.Entries = append(legacy.Entries, corpusfreeze.AssignmentEntry{
			ID: entry.ID, Role: entry.Role, Domain: entry.Domain, SafetyImpact: entry.SafetyImpact,
			SourceID: entry.SourceID, RequirementFile: entry.RequirementFile,
		})
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		return corpusfreeze.Bundle{}, err
	}
	return corpusfreeze.LoadBundle(root, data)
}

func verifyManifest(root, name string) (map[string]string, error) {
	data, err := readRegularFileUnder(root, name)
	if err != nil {
		return nil, fmt.Errorf("read checksum manifest %s: %w", name, err)
	}
	return verifyManifestData(root, name, data)
}

func verifyManifestData(root, name string, data []byte) (map[string]string, error) {
	result := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), maximumPacketFileBytes)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) <= 66 || line[64:66] != "  " {
			return nil, fmt.Errorf("checksum manifest %s contains malformed entry", name)
		}
		digest, relative := line[:64], line[66:]
		if !validSHA256(digest) || !validRelativePath(relative) || result[relative] != "" {
			return nil, fmt.Errorf("checksum manifest %s contains invalid path or hash", name)
		}
		actualData, err := readRegularFileUnder(root, relative)
		if err != nil {
			return nil, err
		}
		if hashBytes(actualData) != digest {
			return nil, fmt.Errorf("checksum manifest %s hash mismatch for %s", name, relative)
		}
		result[relative] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("checksum manifest %s is empty", name)
	}
	return result, nil
}

func readRegularFileUnder(root, relative string) ([]byte, error) {
	if !validRelativePath(relative) {
		return nil, fmt.Errorf("unsafe relative path")
	}
	current := root
	for _, segment := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("path contains symlink")
		}
	}
	info, err := os.Lstat(current)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumPacketFileBytes {
		return nil, fmt.Errorf("path is not a bounded regular file")
	}
	file, err := os.Open(current)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("path changed during open")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumPacketFileBytes+1))
	if err != nil || len(data) > maximumPacketFileBytes {
		return nil, fmt.Errorf("read bounded file: %w", err)
	}
	return data, nil
}

func validRelativePath(value string) bool {
	return value != "" && value != "." && strings.TrimSpace(value) == value && !strings.ContainsAny(value, `\:`) &&
		!path.IsAbs(value) && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
