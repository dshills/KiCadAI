package sexpr

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestRenderAtom(t *testing.T) {
	got, err := Format(A("BUS[0..7]"))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if got != "BUS[0..7]\n" {
		t.Fatalf("Format = %q", got)
	}
}

func TestRenderRejectsInvalidAtom(t *testing.T) {
	got, err := Format(A("1e10"))
	if !errors.Is(err, ErrInvalidAtom) {
		t.Fatalf("error = %v, want ErrInvalidAtom", err)
	}
	if got != "" {
		t.Fatalf("Format returned partial output %q", got)
	}
}

func TestRenderStringEscaping(t *testing.T) {
	got, err := Format(S("quote \" slash \\ newline\n tab\t control\x01"))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	want := "\"quote \\\" slash \\\\ newline\\n tab\\t control\\x01\"\n"
	if got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
}

func TestRenderNestedListUsesStableIndentation(t *testing.T) {
	got, err := Format(L(
		A("kicad_sch"),
		L(A("version"), I(20230121)),
		L(A("generator"), S("kicadai")),
		L(A("paper"), A("A4")),
	))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	want := strings.Join([]string{
		"(kicad_sch",
		"  (version 20230121)",
		"  (generator \"kicadai\")",
		"  (paper A4)",
		")",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Format =\n%s\nwant =\n%s", got, want)
	}
}

func TestRenderFloatFormatting(t *testing.T) {
	tests := []struct {
		name  string
		value Float
		want  string
	}{
		{name: "plain", value: F(1.25), want: "1.25\n"},
		{name: "negative zero", value: F(-0.0), want: "0\n"},
		{name: "fixed not exponent", value: F(0.00000012), want: "0.00000012\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Format(test.value)
			if err != nil {
				t.Fatalf("Format returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("Format = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderRejectsInvalidFloat(t *testing.T) {
	_, err := Format(F(math.Inf(1)))
	if !errors.Is(err, ErrInvalidFloat) {
		t.Fatalf("error = %v, want ErrInvalidFloat", err)
	}
}

func TestRenderFixedNumeric(t *testing.T) {
	got, err := Format(X("1.0"))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if got != "1.0\n" {
		t.Fatalf("Format = %q", got)
	}
}

func TestRenderRawFragment(t *testing.T) {
	got, err := Format(L(A("root"), R(`(embedded_fonts no)`)))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}
	if !strings.Contains(got, "(embedded_fonts no)") {
		t.Fatalf("Format = %q", got)
	}
}

func TestRenderRejectsInvalidRawFragment(t *testing.T) {
	_, err := Format(R(`(embedded_fonts no))`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderRejectsInvalidFixedNumeric(t *testing.T) {
	_, err := Format(X("1e10"))
	if !errors.Is(err, ErrInvalidFixed) {
		t.Fatalf("error = %v, want ErrInvalidFixed", err)
	}
}

func TestRenderOmitsOptionalNodes(t *testing.T) {
	got, err := Format(L(A("symbol"), Omit{}, L(A("uuid"), S("abc"))))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	want := strings.Join([]string{
		"(symbol",
		"  (uuid \"abc\")",
		")",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Format =\n%s\nwant =\n%s", got, want)
	}
}

func TestRenderGoldenSchematicFragment(t *testing.T) {
	got, err := Format(L(
		A("symbol"),
		L(A("lib_id"), S("Device:R")),
		L(A("at"), X("25.4"), X("50.8"), I(0)),
		L(A("property"), S("Reference"), S("R1"), L(A("at"), X("25.4"), X("48.26"), I(0))),
	))
	if err != nil {
		t.Fatalf("Format returned error: %v", err)
	}

	want := strings.Join([]string{
		"(symbol",
		"  (lib_id \"Device:R\")",
		"  (at 25.4 50.8 0)",
		"  (property",
		"    \"Reference\"",
		"    \"R1\"",
		"    (at 25.4 48.26 0)",
		"  )",
		")",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("Format =\n%s\nwant =\n%s", got, want)
	}
}

func TestParseNestedListWithStringsAndComments(t *testing.T) {
	node, err := Parse([]byte(`; comment
(kicad_sch (version 20260306) (generator "eeschema") (paper A4) (uuid "abc"))`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if node.Head() != "kicad_sch" {
		t.Fatalf("head = %q", node.Head())
	}
	version, ok := node.Child("version")
	if !ok || version.ListValue(1) != "20260306" {
		t.Fatalf("version = %#v", version)
	}
	generator, ok := node.Child("generator")
	if !ok || generator.ListValue(1) != "eeschema" {
		t.Fatalf("generator = %#v", generator)
	}
	if !strings.Contains(node.Raw, "(paper A4)") {
		t.Fatalf("raw not preserved: %q", node.Raw)
	}
}

func TestParseReportsUsefulMalformedError(t *testing.T) {
	_, err := Parse([]byte(`(kicad_pcb (version 1)`))
	if err == nil || !strings.Contains(err.Error(), "offset") {
		t.Fatalf("expected offset error, got %v", err)
	}
}

func TestParseRejectsExcessiveNesting(t *testing.T) {
	input := strings.Repeat("(", maxParseDepth+2) + strings.Repeat(")", maxParseDepth+2)
	_, err := Parse([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "maximum list nesting") {
		t.Fatalf("expected nesting error, got %v", err)
	}
}

func TestParsedNodeConvertsEmptyListToListNode(t *testing.T) {
	node, err := Parse([]byte(`()`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := node.Node().(List); !ok {
		t.Fatalf("Node() = %T, want List", node.Node())
	}
}

func TestParseAtomHandlesUTF8(t *testing.T) {
	node, err := Parse([]byte(`(root café)`))
	if err != nil {
		t.Fatal(err)
	}
	if node.ListValue(1) != "café" {
		t.Fatalf("atom = %q", node.ListValue(1))
	}
}
