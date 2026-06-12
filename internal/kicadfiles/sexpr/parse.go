package sexpr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxParseDepth = 500

type ParsedNode struct {
	Atom     string
	String   string
	Children []ParsedNode
	Raw      string
	Quoted   bool
	IsList   bool
}

func Parse(data []byte) (ParsedNode, error) {
	parser := sexprParser{input: string(data)}
	parser.skipSpace()
	node, err := parser.parseNode(0)
	if err != nil {
		return ParsedNode{}, err
	}
	parser.skipSpace()
	if !parser.eof() {
		return ParsedNode{}, parser.errorf("unexpected trailing input")
	}
	return node, nil
}

func (node ParsedNode) Head() string {
	if len(node.Children) == 0 {
		return ""
	}
	return node.Children[0].Value()
}

func (node ParsedNode) Value() string {
	if node.Quoted {
		return node.String
	}
	return node.Atom
}

func (node ParsedNode) Child(head string) (ParsedNode, bool) {
	for _, child := range node.Children {
		if child.Head() == head {
			return child, true
		}
	}
	return ParsedNode{}, false
}

func (node ParsedNode) ChildrenByHead(head string) []ParsedNode {
	var result []ParsedNode
	for _, child := range node.Children {
		if child.Head() == head {
			result = append(result, child)
		}
	}
	return result
}

func (node ParsedNode) ListValue(index int) string {
	if index < 0 || index >= len(node.Children) {
		return ""
	}
	return node.Children[index].Value()
}

func (node ParsedNode) FloatValue(index int) (float64, bool) {
	value := node.ListValue(index)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil
}

func (node ParsedNode) Node() Node {
	if node.IsList {
		nodes := make([]Node, 0, len(node.Children))
		for _, child := range node.Children {
			nodes = append(nodes, child.Node())
		}
		return List(nodes)
	}
	if node.Quoted {
		return String(node.String)
	}
	return Atom(node.Atom)
}

type sexprParser struct {
	input string
	pos   int
}

func (parser *sexprParser) parseNode(depth int) (ParsedNode, error) {
	if depth > maxParseDepth {
		return ParsedNode{}, parser.errorf("maximum list nesting exceeded")
	}
	parser.skipSpace()
	if parser.eof() {
		return ParsedNode{}, parser.errorf("unexpected end of input")
	}
	switch parser.input[parser.pos] {
	case '(':
		return parser.parseList(depth)
	case '"':
		return parser.parseString()
	default:
		return parser.parseAtom()
	}
}

func (parser *sexprParser) parseList(depth int) (ParsedNode, error) {
	start := parser.pos
	parser.pos++
	var children []ParsedNode
	for {
		parser.skipSpace()
		if parser.eof() {
			return ParsedNode{}, parser.errorf("unterminated list")
		}
		if parser.input[parser.pos] == ')' {
			parser.pos++
			return ParsedNode{Children: children, Raw: parser.input[start:parser.pos], IsList: true}, nil
		}
		child, err := parser.parseNode(depth + 1)
		if err != nil {
			return ParsedNode{}, err
		}
		children = append(children, child)
	}
}

func (parser *sexprParser) parseString() (ParsedNode, error) {
	start := parser.pos
	parser.pos++
	var builder strings.Builder
	for !parser.eof() {
		ch := parser.input[parser.pos]
		parser.pos++
		switch ch {
		case '"':
			return ParsedNode{String: builder.String(), Raw: parser.input[start:parser.pos], Quoted: true}, nil
		case '\\':
			if parser.eof() {
				return ParsedNode{}, parser.errorf("unterminated string escape")
			}
			esc := parser.input[parser.pos]
			parser.pos++
			switch esc {
			case 'n':
				builder.WriteByte('\n')
			case 't':
				builder.WriteByte('\t')
			case '"', '\\':
				builder.WriteByte(esc)
			case 'x':
				if parser.pos+2 > len(parser.input) {
					return ParsedNode{}, parser.errorf("short hex escape")
				}
				value, err := strconv.ParseUint(parser.input[parser.pos:parser.pos+2], 16, 8)
				if err != nil {
					return ParsedNode{}, parser.errorf("invalid hex escape")
				}
				builder.WriteByte(byte(value))
				parser.pos += 2
			default:
				builder.WriteByte(esc)
			}
		default:
			builder.WriteByte(ch)
		}
	}
	return ParsedNode{}, parser.errorf("unterminated string")
}

func (parser *sexprParser) parseAtom() (ParsedNode, error) {
	start := parser.pos
	for !parser.eof() {
		ch, size := utf8.DecodeRuneInString(parser.input[parser.pos:])
		if unicode.IsSpace(ch) || ch == '(' || ch == ')' || ch == ';' {
			break
		}
		parser.pos += size
	}
	if parser.pos == start {
		return ParsedNode{}, parser.errorf("expected atom")
	}
	return ParsedNode{Atom: parser.input[start:parser.pos], Raw: parser.input[start:parser.pos]}, nil
}

func (parser *sexprParser) skipSpace() {
	for !parser.eof() {
		ch, size := utf8.DecodeRuneInString(parser.input[parser.pos:])
		if unicode.IsSpace(ch) {
			parser.pos += size
			continue
		}
		if ch == ';' {
			for !parser.eof() && parser.input[parser.pos] != '\n' {
				parser.pos++
			}
			continue
		}
		return
	}
}

func (parser sexprParser) eof() bool {
	return parser.pos >= len(parser.input)
}

func (parser sexprParser) errorf(message string) error {
	return fmt.Errorf("sexpr parse error at offset %d: %s", parser.pos, message)
}
