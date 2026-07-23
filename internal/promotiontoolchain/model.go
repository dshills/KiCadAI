package promotiontoolchain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	LockSchema     = "kicadai.kicad-promotion-toolchain.v1"
	EvidenceSchema = "kicadai.resolved-kicad-toolchain.v1"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Environment struct {
	KiCadCLI       string `json:"kicad_cli"`
	SymbolsRoot    string `json:"symbols_root"`
	FootprintsRoot string `json:"footprints_root"`
}

type Bootstrap struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type Platform struct {
	OS             string    `json:"os"`
	Arch           string    `json:"arch"`
	TrustedRoots   []string  `json:"trusted_roots"`
	KiCadCLI       string    `json:"kicad_cli"`
	SymbolsRoot    string    `json:"symbols_root"`
	FootprintsRoot string    `json:"footprints_root"`
	SymbolTable    string    `json:"symbol_table"`
	FootprintTable string    `json:"footprint_table"`
	Bootstrap      Bootstrap `json:"bootstrap"`
}

type Lock struct {
	Schema       string      `json:"schema"`
	Version      int         `json:"version"`
	KiCadVersion string      `json:"kicad_version"`
	Environment  Environment `json:"environment"`
	Platforms    []Platform  `json:"platforms"`
}

type Document struct {
	Lock   Lock
	Path   string
	SHA256 string
}

type LibraryIdentity struct {
	SHA256    string `json:"sha256"`
	FileCount int    `json:"file_count"`
	ByteCount int64  `json:"byte_count"`
}

type Evidence struct {
	Schema               string          `json:"schema"`
	Version              int             `json:"version"`
	LockSHA256           string          `json:"lock_sha256"`
	OS                   string          `json:"os"`
	Arch                 string          `json:"arch"`
	KiCadVersion         string          `json:"kicad_version"`
	KiCadCLI             string          `json:"kicad_cli"`
	SymbolsRoot          string          `json:"symbols_root"`
	FootprintsRoot       string          `json:"footprints_root"`
	SymbolTable          string          `json:"symbol_table"`
	FootprintTable       string          `json:"footprint_table"`
	SymbolTableSHA256    string          `json:"symbol_table_sha256"`
	FootprintTableSHA256 string          `json:"footprint_table_sha256"`
	SymbolsIdentity      LibraryIdentity `json:"symbols_identity"`
	FootprintsIdentity   LibraryIdentity `json:"footprints_identity"`
	Resolution           string          `json:"resolution"`
}

func Load(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read toolchain lock: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var lock Lock
	if err := decoder.Decode(&lock); err != nil {
		return Document{}, fmt.Errorf("decode toolchain lock: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Document{}, err
	}
	if err := validateLock(lock); err != nil {
		return Document{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Document{}, fmt.Errorf("resolve toolchain lock path: %w", err)
	}
	sum := sha256.Sum256(data)
	return Document{Lock: lock, Path: absolute, SHA256: hex.EncodeToString(sum[:])}, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode toolchain lock: multiple JSON values")
		}
		return fmt.Errorf("decode toolchain lock trailer: %w", err)
	}
	return nil
}

func validateLock(lock Lock) error {
	if lock.Schema != LockSchema || lock.Version != 1 {
		return fmt.Errorf("unsupported toolchain lock schema %q version %d", lock.Schema, lock.Version)
	}
	if !validVersion(lock.KiCadVersion) {
		return fmt.Errorf("invalid locked KiCad version %q", lock.KiCadVersion)
	}
	for name, value := range map[string]string{
		"environment.kicad_cli":       lock.Environment.KiCadCLI,
		"environment.symbols_root":    lock.Environment.SymbolsRoot,
		"environment.footprints_root": lock.Environment.FootprintsRoot,
	} {
		if !validEnvironmentName(value) {
			return fmt.Errorf("%s must be an explicit environment variable name", name)
		}
	}
	if len(lock.Platforms) == 0 {
		return errors.New("toolchain lock must declare at least one platform")
	}
	keys := make([]string, 0, len(lock.Platforms))
	for index, platform := range lock.Platforms {
		if err := validatePlatform(platform); err != nil {
			return fmt.Errorf("platforms[%d]: %w", index, err)
		}
		keys = append(keys, platform.OS+"/"+platform.Arch)
	}
	sorted := slices.Clone(keys)
	slices.Sort(sorted)
	for index := 1; index < len(sorted); index++ {
		if sorted[index] == sorted[index-1] {
			return fmt.Errorf("duplicate platform %q", sorted[index])
		}
	}
	return nil
}

func validatePlatform(platform Platform) error {
	if strings.TrimSpace(platform.OS) == "" || strings.TrimSpace(platform.Arch) == "" {
		return errors.New("os and arch are required")
	}
	if len(platform.TrustedRoots) == 0 {
		return errors.New("at least one trusted root is required")
	}
	for _, root := range platform.TrustedRoots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return fmt.Errorf("trusted root %q must be a clean absolute path", root)
		}
	}
	for name, value := range map[string]string{
		"kicad_cli":       platform.KiCadCLI,
		"symbols_root":    platform.SymbolsRoot,
		"footprints_root": platform.FootprintsRoot,
		"symbol_table":    platform.SymbolTable,
		"footprint_table": platform.FootprintTable,
	} {
		if !safeRelativePath(value) {
			return fmt.Errorf("%s %q must be a clean relative path", name, value)
		}
	}
	if platform.Bootstrap.Kind != "dmg" {
		return fmt.Errorf("unsupported bootstrap kind %q", platform.Bootstrap.Kind)
	}
	parsed, err := url.Parse(platform.Bootstrap.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("bootstrap URL %q must be absolute HTTPS", platform.Bootstrap.URL)
	}
	if !sha256Pattern.MatchString(platform.Bootstrap.SHA256) {
		return errors.New("bootstrap sha256 must be 64 lowercase hexadecimal characters")
	}
	if platform.Bootstrap.SizeBytes <= 0 {
		return errors.New("bootstrap size_bytes must be positive")
	}
	return nil
}

func validVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func validEnvironmentName(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if (r >= 'A' && r <= 'Z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func safeRelativePath(value string) bool {
	return value != "" && !filepath.IsAbs(value) && filepath.Clean(value) == value && value != "." &&
		value != ".." && !strings.HasPrefix(value, ".."+string(filepath.Separator))
}

func (document Document) Platform(osName, arch string) (Platform, error) {
	for _, platform := range document.Lock.Platforms {
		if platform.OS == osName && platform.Arch == arch {
			return platform, nil
		}
	}
	return Platform{}, fmt.Errorf("locked KiCad toolchain does not support %s/%s", osName, arch)
}
