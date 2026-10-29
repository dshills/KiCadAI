// Package evidencebundle publishes bounded, portable diagnostic evidence.
// Hashes detect changes; they are not signatures or independent attestations.
package evidencebundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"kicadai/internal/atomicdir"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"
)

const MaxFile = 2 << 20
const MaxTotal = 32 << 20

// MaxFiles includes manifest.json, allowing at most 255 payload files.
const MaxFiles = 256

type Identity struct {
	Go               string `json:"go"`
	Revision         string `json:"revision"`
	Modified         string `json:"modified"`
	ExecutableSHA256 string `json:"executable_sha256"`
}

func CurrentIdentity() Identity {
	value := Identity{Revision: "unknown", Modified: "unknown"}
	if info, ok := debug.ReadBuildInfo(); ok {
		value.Go = info.GoVersion
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				value.Revision = setting.Value
			case "vcs.modified":
				value.Modified = setting.Value
			}
		}
	}
	if path, err := os.Executable(); err == nil {
		value.ExecutableSHA256, _ = FileDigest(path)
	}
	return value
}

func Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func FileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 512<<20 {
		return "", errors.New("executable is not a regular file within 512 MiB")
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, (512<<20)+1))
	if err != nil {
		return "", err
	}
	if count > 512<<20 {
		return "", errors.New("executable exceeds size limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func CheckDestination(output string) error {
	if output == "" {
		return errors.New("new output directory is required")
	}
	if _, err := os.Lstat(output); err == nil {
		return errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}
func JSON(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	return append(raw, '\n'), err
}
func Decode(raw []byte, value any) error {
	if len(raw) > MaxFile {
		return errors.New("evidence input exceeds 2 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}
func Read(root, name string) ([]byte, error) {
	if !portablePath(name) {
		return nil, errors.New("invalid evidence path")
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFile {
		return nil, errors.New("evidence must be a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, MaxFile+1))
	if len(raw) > MaxFile {
		return nil, errors.New("evidence grew beyond size limit")
	}
	return raw, err
}

func Publish(output string, files map[string][]byte) error {
	if len(files) == 0 || len(files) >= MaxFiles {
		return errors.New("invalid evidence file count")
	}
	total := 0
	manifest := map[string]string{}
	for name, raw := range files {
		if !portablePath(name) || name == "manifest.json" || len(raw) > MaxFile {
			return errors.New("invalid evidence name or size")
		}
		total += len(raw)
		manifest[name] = Digest(raw)
	}
	if total > MaxTotal {
		return errors.New("evidence exceeds 32 MiB")
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	return atomicdir.Publish(output, func(stage string) error {
		for name, raw := range files {
			path := filepath.Join(stage, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				return err
			}
		}
		raw, err := JSON(manifest)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage, "manifest.json"), raw, 0600)
	})
}

func Verify(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("evidence root must be a real directory")
	}
	raw, err := Read(root, "manifest.json")
	if err != nil {
		return err
	}
	var expected map[string]string
	if err = Decode(raw, &expected); err != nil {
		return err
	}
	if len(expected) == 0 || len(expected) >= MaxFiles {
		return errors.New("invalid manifest count")
	}
	actual := map[string]string{}
	total := 0
	entries := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > MaxFiles*4 {
			return errors.New("too many evidence directories or files")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("non-regular evidence artifact")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if name == "manifest.json" {
			return nil
		}
		if len(actual) >= MaxFiles {
			return errors.New("too many evidence artifacts")
		}
		data, err := Read(root, name)
		if err != nil {
			return err
		}
		total += len(data)
		if total > MaxTotal {
			return errors.New("evidence too large")
		}
		actual[name] = Digest(data)
		return nil
	})
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("evidence inventory or digest differs")
	}
	return nil
}

func portablePath(name string) bool {
	return name != "." && fs.ValidPath(name) && !strings.ContainsAny(name, `\:`) && len(name) <= 256 && strings.Count(name, "/") <= 4
}
