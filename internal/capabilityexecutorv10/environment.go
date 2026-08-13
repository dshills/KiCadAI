package capabilityexecutorv10

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"kicadai/internal/promotiontoolchain"
)

const maximumToolBytes int64 = 1024 * 1024 * 1024

// PromotionEnvironmentHash commits the resolved, content-addressed promotion
// environment without embedding host-specific absolute paths.
func PromotionEnvironmentHash(evidence promotiontoolchain.Evidence) (string, error) {
	type commitment struct {
		Schema               string                             `json:"schema"`
		Version              int                                `json:"version"`
		LockSHA256           string                             `json:"lock_sha256"`
		OS                   string                             `json:"os"`
		Arch                 string                             `json:"arch"`
		KiCadVersion         string                             `json:"kicad_version"`
		SymbolTableSHA256    string                             `json:"symbol_table_sha256"`
		FootprintTableSHA256 string                             `json:"footprint_table_sha256"`
		SymbolsIdentity      promotiontoolchain.LibraryIdentity `json:"symbols_identity"`
		FootprintsIdentity   promotiontoolchain.LibraryIdentity `json:"footprints_identity"`
		Resolution           string                             `json:"resolution"`
	}
	if !digestPattern.MatchString(evidence.LockSHA256) || !digestPattern.MatchString(evidence.SymbolTableSHA256) ||
		!digestPattern.MatchString(evidence.FootprintTableSHA256) || !digestPattern.MatchString(evidence.SymbolsIdentity.SHA256) ||
		!digestPattern.MatchString(evidence.FootprintsIdentity.SHA256) || evidence.SymbolsIdentity.FileCount <= 0 ||
		evidence.FootprintsIdentity.FileCount <= 0 || evidence.SymbolsIdentity.ByteCount <= 0 || evidence.FootprintsIdentity.ByteCount <= 0 {
		return "", fmt.Errorf("promotion environment evidence is incomplete")
	}
	data, err := json.Marshal(commitment{
		Schema: evidence.Schema, Version: evidence.Version, LockSHA256: evidence.LockSHA256, OS: evidence.OS, Arch: evidence.Arch,
		KiCadVersion: evidence.KiCadVersion, SymbolTableSHA256: evidence.SymbolTableSHA256,
		FootprintTableSHA256: evidence.FootprintTableSHA256, SymbolsIdentity: evidence.SymbolsIdentity,
		FootprintsIdentity: evidence.FootprintsIdentity, Resolution: evidence.Resolution,
	})
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

// RegularFileSHA256 binds the exact installed executable while rejecting
// symlinks, empty files, oversized files, and path replacement races.
func RegularFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() <= 0 || opened.Size() > maximumToolBytes {
		return "", fmt.Errorf("tool is not a bounded nonempty regular file")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(opened, pathInfo) {
		return "", fmt.Errorf("tool path changed or is not a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
