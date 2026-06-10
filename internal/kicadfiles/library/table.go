package library

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

var nicknamePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+-]*$`)

type TableEntry struct {
	Name        string
	Type        string
	URI         string
	Options     string
	Description string
}

func WriteSymbolLibraryTable(w io.Writer, entries []TableEntry) error {
	return writeTable(w, "sym_lib_table", "sym_lib_table", entries)
}

func WriteFootprintLibraryTable(w io.Writer, entries []TableEntry) error {
	return writeTable(w, "fp_lib_table", "fp_lib_table", entries)
}

func ValidateTableEntries(section string, entries []TableEntry) error {
	var errs kicadfiles.ValidationErrors
	seen := map[string]struct{}{}
	for i, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			errs = append(errs, fieldError(section, i, "name", "required"))
		} else {
			if !nicknamePattern.MatchString(name) {
				errs = append(errs, fieldError(section, i, "name", "invalid library nickname"))
			}
			folded := strings.ToLower(name)
			if _, ok := seen[folded]; ok {
				errs = append(errs, fieldError(section, i, "name", "duplicate "+name))
			}
			seen[folded] = struct{}{}
		}
		if strings.TrimSpace(entry.Type) == "" {
			errs = append(errs, fieldError(section, i, "type", "required"))
		}
		if strings.TrimSpace(entry.URI) == "" {
			errs = append(errs, fieldError(section, i, "uri", "required"))
		}
	}
	return errs.Err()
}

func writeTable(w io.Writer, root, section string, entries []TableEntry) error {
	if err := ValidateTableEntries(section, entries); err != nil {
		return err
	}
	nodes := []sexpr.Node{sexpr.A(root), sexpr.L(sexpr.A("version"), sexpr.I(7))}
	for _, entry := range entries {
		nodes = append(nodes, sexpr.L(
			sexpr.A("lib"),
			sexpr.L(sexpr.A("name"), sexpr.S(strings.TrimSpace(entry.Name))),
			sexpr.L(sexpr.A("type"), sexpr.S(strings.TrimSpace(entry.Type))),
			sexpr.L(sexpr.A("uri"), sexpr.S(strings.TrimSpace(entry.URI))),
			sexpr.L(sexpr.A("options"), sexpr.S(entry.Options)),
			sexpr.L(sexpr.A("descr"), sexpr.S(entry.Description)),
		))
	}
	return sexpr.Render(w, sexpr.L(nodes...))
}

func fieldError(section string, index int, field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{
		Section: section,
		Field:   fmt.Sprintf("entries[%d].%s", index, field),
		Message: message,
	}
}
