package sexpr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidAtom  = errors.New("invalid atom")
	ErrInvalidFixed = errors.New("invalid fixed numeric literal")
	ErrInvalidFloat = errors.New("invalid float")
	ErrInvalidNode  = errors.New("invalid node")
)

var (
	atomPattern    = regexp.MustCompile(`^[A-Za-z0-9_+*/<>=!?$%&~^:.@#\[\]{}-]+$`)
	numericPattern = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)
	fixedPattern   = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$`)
)

const indentSpaces = "                                                                "
const upperHex = "0123456789ABCDEF"

type Node interface{}

type Atom string
type String string
type Int int64
type Float float64
type Fixed string
type Raw string
type List []Node
type Omit struct{}

func A(value string) Atom {
	return Atom(value)
}

func S(value string) String {
	return String(value)
}

func I(value int64) Int {
	return Int(value)
}

func F(value float64) Float {
	return Float(value)
}

func X(value string) Fixed {
	return Fixed(value)
}

func R(value string) Raw {
	return Raw(value)
}

func L(nodes ...Node) List {
	return List(nodes)
}

func OmitIf(condition bool, node Node) Node {
	if condition {
		return Omit{}
	}
	return node
}

func Format(node Node) (string, error) {
	var buf bytes.Buffer
	if err := Render(&buf, node); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func Render(w io.Writer, node Node) error {
	r := renderer{w: w}
	if isOmit(node) {
		return fmt.Errorf("%w: top-level omit", ErrInvalidNode)
	}
	if err := r.writeNode(node, 0); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

type renderer struct {
	w io.Writer
}

func (r renderer) writeNode(node Node, indent int) error {
	switch value := node.(type) {
	case Atom:
		return r.writeAtom(string(value))
	case String:
		_, err := io.WriteString(r.w, quoteString(string(value)))
		return err
	case Int:
		_, err := io.WriteString(r.w, strconv.FormatInt(int64(value), 10))
		return err
	case Float:
		text, err := formatFloat(float64(value))
		if err != nil {
			return err
		}
		_, err = io.WriteString(r.w, text)
		return err
	case Fixed:
		return r.writeFixed(string(value))
	case Raw:
		raw := strings.TrimSpace(string(value))
		if !ValidRaw(raw) {
			return fmt.Errorf("%w: invalid raw fragment", ErrInvalidNode)
		}
		_, err := io.WriteString(r.w, raw)
		return err
	case List:
		return r.writeList(value, indent)
	default:
		return fmt.Errorf("%w: %T", ErrInvalidNode, node)
	}
}

func ValidRaw(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "(") || !strings.HasSuffix(value, ")") {
		return false
	}
	depth := 0
	inString := false
	escaped := false
	for _, r := range value {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && !inString && !escaped
}

func (r renderer) writeList(list List, indent int) error {
	if visibleCount(list) == 0 {
		_, err := io.WriteString(r.w, "()")
		return err
	}
	if listIsFlat(list) {
		if _, err := io.WriteString(r.w, "("); err != nil {
			return err
		}
		written := 0
		for _, node := range list {
			if isOmit(node) {
				continue
			}
			if written > 0 {
				if _, err := io.WriteString(r.w, " "); err != nil {
					return err
				}
			}
			if err := r.writeNode(node, indent); err != nil {
				return err
			}
			written++
		}
		_, err := io.WriteString(r.w, ")")
		return err
	}

	if _, err := io.WriteString(r.w, "("); err != nil {
		return err
	}
	skippedFirst := false
	firstHandled := false
	for _, node := range list {
		if isOmit(node) {
			continue
		}
		if !firstHandled {
			firstHandled = true
			if isScalar(node) {
				if err := r.writeNode(node, indent); err != nil {
					return err
				}
				skippedFirst = true
				continue
			}
		}
		break
	}
	for _, node := range list {
		if isOmit(node) {
			continue
		}
		if skippedFirst {
			skippedFirst = false
			continue
		}
		if err := writeIndentedNewline(r.w, indent+2); err != nil {
			return err
		}
		if err := r.writeNode(node, indent+2); err != nil {
			return err
		}
	}
	if err := writeIndentedNewline(r.w, indent); err != nil {
		return err
	}
	_, err := io.WriteString(r.w, ")")
	return err
}

func (r renderer) writeAtom(value string) error {
	if !ValidAtom(value) {
		return fmt.Errorf("%w: %q", ErrInvalidAtom, value)
	}
	_, err := io.WriteString(r.w, value)
	return err
}

func (r renderer) writeFixed(value string) error {
	if !fixedPattern.MatchString(value) {
		return fmt.Errorf("%w: %q", ErrInvalidFixed, value)
	}
	_, err := io.WriteString(r.w, value)
	return err
}

func ValidAtom(value string) bool {
	return atomPattern.MatchString(value) && !numericPattern.MatchString(value)
}

func formatFloat(value float64) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("%w: %v", ErrInvalidFloat, value)
	}
	if value == 0 {
		return "0", nil
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func quoteString(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r >= 0 && r < 0x20 {
				b.WriteString(`\x`)
				b.WriteByte(upperHex[r>>4])
				b.WriteByte(upperHex[r&0xF])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func visibleCount(list List) int {
	count := 0
	for _, node := range list {
		if !isOmit(node) {
			count++
		}
	}
	return count
}

func listIsFlat(list List) bool {
	for _, node := range list {
		if isOmit(node) {
			continue
		}
		if !isScalar(node) {
			return false
		}
	}
	return true
}

func isScalar(node Node) bool {
	switch node.(type) {
	case Atom, String, Int, Float, Fixed:
		return true
	default:
		return false
	}
}

func isOmit(node Node) bool {
	_, ok := node.(Omit)
	return ok
}

func writeIndentedNewline(w io.Writer, indent int) error {
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	for indent > 0 {
		chunk := indent
		if chunk > len(indentSpaces) {
			chunk = len(indentSpaces)
		}
		if _, err := io.WriteString(w, indentSpaces[:chunk]); err != nil {
			return err
		}
		indent -= chunk
	}
	return nil
}
