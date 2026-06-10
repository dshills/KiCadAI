package roundtrip

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
)

type Summary struct {
	Sections map[string]int
}

func SummarizePCB(path string) (Summary, error) {
	file, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("open PCB summary input: %w", err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	sections := map[string]int{}
	depth := 0
	for {
		b, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Summary{}, fmt.Errorf("read PCB summary input: %w", err)
		}
		switch b {
		case '(':
			depth++
			if depth == 2 {
				atom, err := readAtom(reader)
				if err != nil {
					return Summary{}, err
				}
				if atom != "" {
					sections[classifyPCBSection(atom)]++
				}
			}
		case ')':
			if depth > 0 {
				depth--
			}
		case '"':
			if err := skipString(reader); err != nil {
				return Summary{}, err
			}
		case ';':
			if err := skipComment(reader); err != nil {
				return Summary{}, err
			}
		}
	}
	return Summary{Sections: sections}, nil
}

func (s Summary) String() string {
	keys := make([]string, 0, len(s.Sections))
	for key := range s.Sections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, key := range keys {
		out.WriteString(fmt.Sprintf("%s=%d\n", key, s.Sections[key]))
	}
	return out.String()
}

func classifyPCBSection(atom string) string {
	switch {
	case strings.HasPrefix(atom, "gr_"):
		return "board_graphics"
	case atom == "segment" || atom == "arc" || atom == "via":
		return "tracks_and_vias"
	default:
		return atom
	}
}

func readAtom(reader *bufio.Reader) (string, error) {
	var atom strings.Builder
	for {
		b, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			return atom.String(), nil
		}
		if err != nil {
			return "", fmt.Errorf("read atom: %w", err)
		}
		if unicode.IsSpace(rune(b)) {
			if atom.Len() == 0 {
				continue
			}
			return atom.String(), nil
		}
		if b == ';' {
			if err := skipComment(reader); err != nil {
				return "", err
			}
			if atom.Len() == 0 {
				continue
			}
			return atom.String(), nil
		}
		if b == '(' || b == ')' {
			if err := reader.UnreadByte(); err != nil {
				return "", fmt.Errorf("unread atom delimiter: %w", err)
			}
			return atom.String(), nil
		}
		atom.WriteByte(b)
	}
}

func skipString(reader *bufio.Reader) error {
	escaped := false
	for {
		b, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("unexpected EOF in string")
		}
		if err != nil {
			return fmt.Errorf("read string: %w", err)
		}
		if escaped {
			escaped = false
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '"':
			return nil
		}
	}
}

func skipComment(reader *bufio.Reader) error {
	for {
		b, err := reader.ReadByte()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read comment: %w", err)
		}
		if b == '\n' || b == '\r' {
			return nil
		}
	}
}
