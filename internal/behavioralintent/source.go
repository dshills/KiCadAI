package behavioralintent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PrepareSource owns prompt segmentation so provider output can refer to
// stable statements without inventing or miscounting byte offsets.
func PrepareSource(prompt string) Source {
	text := strings.TrimSpace(prompt)
	hash := sha256.Sum256([]byte(text))
	return Source{
		SHA256:     hex.EncodeToString(hash[:]),
		ByteLength: len(text),
		Statements: sourceStatements(text),
	}
}

func sourceStatements(text string) []SourceStatement {
	var statements []SourceStatement
	start := 0
	for index := 0; index < len(text); {
		r, size := utf8.DecodeRuneInString(text[index:])
		end := index + size
		boundary := r == '\n' || r == ';'
		if r == '.' || r == '?' || r == '!' {
			boundary = !decimalPoint(text, index)
		}
		if boundary {
			statements = appendStatement(statements, text, start, end)
			start = end
		}
		index = end
	}
	statements = appendStatement(statements, text, start, len(text))
	for index := range statements {
		statements[index].ID = fmt.Sprintf("statement_%03d", index+1)
	}
	return statements
}

func appendStatement(statements []SourceStatement, text string, start, end int) []SourceStatement {
	for start < end {
		r, size := utf8.DecodeRuneInString(text[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		start += size
	}
	for end > start {
		r, size := utf8.DecodeLastRuneInString(text[start:end])
		if !unicode.IsSpace(r) {
			break
		}
		end -= size
	}
	if start == end {
		return statements
	}
	return append(statements, SourceStatement{Text: text[start:end], StartByte: start, EndByte: end})
}

func decimalPoint(text string, index int) bool {
	return index > 0 && index+1 < len(text) && text[index-1] >= '0' && text[index-1] <= '9' && text[index+1] >= '0' && text[index+1] <= '9'
}
