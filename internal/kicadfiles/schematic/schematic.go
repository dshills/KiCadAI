package schematic

import (
	"io"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type SchematicFile struct {
	Filename   string
	Version    kicadfiles.KiCadFormatVersion
	Generator  string
	UUID       kicadfiles.UUID
	Paper      kicadfiles.Paper
	TitleBlock kicadfiles.TitleBlock
}

func Validate(schematic SchematicFile) error {
	var errs kicadfiles.ValidationErrors
	if schematic.Version == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if _, err := strconv.ParseInt(string(schematic.Version), 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	}
	if strings.TrimSpace(schematic.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if !schematic.UUID.Valid() {
		errs = append(errs, fieldError("uuid", "valid UUID required"))
	}
	if strings.TrimSpace(schematic.Paper.Name) == "" {
		errs = append(errs, fieldError("paper", "required"))
	}
	if len(schematic.TitleBlock.Comments) > 9 {
		errs = append(errs, fieldError("title_block.comments", "at most 9 comments allowed"))
	}
	return errs.Err()
}

func Write(w io.Writer, schematic SchematicFile) error {
	if err := Validate(schematic); err != nil {
		return err
	}
	node, err := render(schematic)
	if err != nil {
		return err
	}
	return sexpr.Render(w, node)
}

func render(schematic SchematicFile) (sexpr.List, error) {
	version, err := versionInt(schematic.Version)
	if err != nil {
		return nil, err
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_sch"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(schematic.Generator))),
		sexpr.L(sexpr.A("uuid"), sexpr.S(string(schematic.UUID))),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(schematic.Paper.Name))),
	}
	if title := renderTitleBlock(schematic.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	return sexpr.L(nodes...), nil
}

func renderTitleBlock(title kicadfiles.TitleBlock) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("title_block")}
	if title.Title != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("title"), sexpr.S(title.Title)))
	}
	if title.Date != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("date"), sexpr.S(title.Date)))
	}
	if title.Revision != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("rev"), sexpr.S(title.Revision)))
	}
	if title.Company != "" {
		nodes = append(nodes, sexpr.L(sexpr.A("company"), sexpr.S(title.Company)))
	}
	for i, comment := range title.Comments {
		nodes = append(nodes, sexpr.L(sexpr.A("comment"), sexpr.I(int64(i+1)), sexpr.S(comment)))
	}
	return sexpr.L(nodes...)
}

func versionInt(version kicadfiles.KiCadFormatVersion) (int64, error) {
	return strconv.ParseInt(string(version), 10, 64)
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "schematic", Field: field, Message: message}
}
