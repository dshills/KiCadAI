package capabilitybaselinepublicationv10

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func canonicalJSON(value any) ([]byte, error) {
	if err := rejectJSONMaps(reflect.TypeOf(value), map[reflect.Type]bool{}); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func withSingleTrailingLF(data []byte) []byte { return append(bytes.TrimRight(data, "\r\n"), '\n') }

func decodeStrict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func manifestHash(value Manifest) (string, error) {
	value.Hash = ""
	if err := rejectJSONMaps(reflect.TypeOf(value), map[reflect.Type]bool{}); err != nil {
		return "", err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func rejectJSONMaps(typeOf reflect.Type, seen map[reflect.Type]bool) error {
	if typeOf == nil || seen[typeOf] {
		return nil
	}
	seen[typeOf] = true
	if typeOf.Kind() == reflect.Map {
		return fmt.Errorf("map-backed JSON is prohibited from V10 baseline artifacts")
	}
	if typeOf.Kind() == reflect.Interface {
		return fmt.Errorf("interface-backed JSON is prohibited from V10 baseline artifacts")
	}
	switch typeOf.Kind() {
	case reflect.Struct:
		for index := 0; index < typeOf.NumField(); index++ {
			if err := rejectJSONMaps(typeOf.Field(index).Type, seen); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice, reflect.Pointer:
		return rejectJSONMaps(typeOf.Elem(), seen)
	}
	return nil
}

func checksumBytes(files map[string][]byte) []byte {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var result strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&result, "%s  %s\n", hashBytes(files[path]), path)
	}
	return []byte(result.String())
}

func validateBinding(value Binding) error {
	current := reflect.ValueOf(value)
	typeOf := current.Type()
	for index := 0; index < current.NumField(); index++ {
		field := typeOf.Field(index)
		if field.Type.Kind() != reflect.String {
			return fmt.Errorf("binding field %s must be a string", field.Name)
		}
		text := current.Field(index).String()
		switch field.Tag.Get("binding") {
		case "commit":
			if !commitPattern.MatchString(text) {
				return fmt.Errorf("binding field %s is not a canonical commit", field.Name)
			}
		case "digest":
			if !digestPattern.MatchString(text) {
				return fmt.Errorf("binding field %s is not a canonical SHA-256 digest", field.Name)
			}
		default:
			return fmt.Errorf("binding field %s has no frozen validation class", field.Name)
		}
	}
	return nil
}
