package schematic

import (
	"kicadai/internal/kicadfiles/sexpr"
	"strings"
	"testing"
)

func TestNativeV10TextReaderRejectsUnmodeledEffects(t *testing.T) {
	source := `(text "purpose" (exclude_from_sim no) (at 20 30 0) (effects (font (size 1.27 1.27))) (uuid "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))`
	n, err := sexpr.Parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if text, ok := readNativeV10Text(n); !ok || !text.NativeV10 || text.Value != "purpose" {
		t.Fatal("supported native text not modeled")
	}
	for _, bad := range []string{"(text)", strings.Replace(source, "1.27 1.27", "2 2", 1), strings.Replace(source, "(font ", "(justify left) (font ", 1), strings.Replace(source, "exclude_from_sim no", "exclude_from_sim yes", 1), strings.Replace(source, "(at 20", "(unknown yes) (at 20", 1)} {
		n, err := sexpr.Parse([]byte(bad))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := readNativeV10Text(n); ok {
			t.Fatal("unmodeled text accepted")
		}
	}
}
