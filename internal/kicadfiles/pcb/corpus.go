package pcb

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type CorpusReport struct {
	Files       int
	ObjectCount map[string]int
}

func ScanCorpus(root string) (CorpusReport, error) {
	report := CorpusReport{ObjectCount: map[string]int{}}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".kicad_pcb") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		counts, scanErr := scanPCBObjects(file)
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		report.Files++
		for object, count := range counts {
			report.ObjectCount[object] += count
		}
		return nil
	})
	return report, err
}

func scanPCBObjects(r io.Reader) (map[string]int, error) {
	counts := map[string]int{}
	reader := bufio.NewReader(r)
	inString := false
	escaped := false
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return counts, nil
		}
		if err != nil {
			return counts, err
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch b {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case ';':
			if err := skipLineComment(reader); err != nil {
				return counts, err
			}
		case '(':
			token, err := readObjectToken(reader)
			if err != nil {
				return counts, err
			}
			if token != "" {
				counts[token]++
			}
		}
	}
}

func skipLineComment(reader *bufio.Reader) error {
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if b == '\n' {
			return nil
		}
	}
}

func readObjectToken(reader *bufio.Reader) (string, error) {
	var token strings.Builder
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return token.String(), nil
		}
		if err != nil {
			return "", err
		}
		r := rune(b)
		if token.Len() == 0 {
			if unicode.IsSpace(r) {
				continue
			}
			if !isObjectTokenStart(r) {
				if err := reader.UnreadByte(); err != nil {
					return "", err
				}
				return "", nil
			}
			token.WriteByte(b)
			continue
		}
		if !isObjectTokenPart(r) {
			if err := reader.UnreadByte(); err != nil {
				return "", err
			}
			return token.String(), nil
		}
		token.WriteByte(b)
	}
}

func isObjectTokenStart(r rune) bool {
	return r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func isObjectTokenPart(r rune) bool {
	return isObjectTokenStart(r) || r >= '0' && r <= '9'
}
