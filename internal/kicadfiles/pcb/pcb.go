package pcb

import (
	"cmp"
	"io"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

type PCBFile struct {
	Version    kicadfiles.KiCadFormatVersion
	Generator  string
	General    PCBGeneral
	Paper      kicadfiles.Paper
	Layers     []LayerDefinition
	Setup      PCBSetup
	Nets       []Net
	TitleBlock kicadfiles.TitleBlock
}

type PCBGeneral struct{}

type PCBSetup struct {
	Stackup            PCBStackup
	SolderMaskMinWidth kicadfiles.IU
	PadToMaskClearance kicadfiles.IU
}

type PCBStackup struct {
	Thickness kicadfiles.IU
}

type LayerDefinition struct {
	Number int
	Name   kicadfiles.BoardLayer
	Kind   string
}

type Net struct {
	Code int
	Name string
}

func DefaultTwoLayerStack() []LayerDefinition {
	return []LayerDefinition{
		{Number: 0, Name: kicadfiles.LayerFCu, Kind: "signal"},
		{Number: 31, Name: kicadfiles.LayerBCu, Kind: "signal"},
		{Number: 36, Name: kicadfiles.LayerBSilkS, Kind: "user"},
		{Number: 37, Name: kicadfiles.LayerFSilkS, Kind: "user"},
		{Number: 44, Name: kicadfiles.LayerEdge, Kind: "user"},
	}
}

func Validate(board PCBFile) error {
	var errs kicadfiles.ValidationErrors
	if board.Version == "" {
		errs = append(errs, fieldError("version", "required"))
	} else if _, err := strconv.ParseInt(string(board.Version), 10, 64); err != nil {
		errs = append(errs, fieldError("version", "must be numeric"))
	}
	if strings.TrimSpace(board.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if strings.TrimSpace(board.Paper.Name) == "" {
		errs = append(errs, fieldError("paper", "required"))
	}
	if board.Setup.Stackup.Thickness <= 0 {
		errs = append(errs, fieldError("setup.stackup.thickness", "must be positive"))
	}
	if board.Setup.SolderMaskMinWidth < 0 {
		errs = append(errs, fieldError("setup.solder_mask_min_width", "must be non-negative"))
	}
	if board.Setup.PadToMaskClearance < 0 {
		errs = append(errs, fieldError("setup.pad_to_mask_clearance", "must be non-negative"))
	}
	if len(board.TitleBlock.Comments) > 9 {
		errs = append(errs, fieldError("title_block.comments", "at most 9 comments allowed"))
	}
	if len(board.Layers) == 0 {
		errs = append(errs, fieldError("layers", "at least one layer required"))
	}
	layerNumbers := make(map[int]struct{}, len(board.Layers))
	layerNames := make(map[kicadfiles.BoardLayer]struct{}, len(board.Layers))
	for i, layer := range board.Layers {
		if _, ok := layerNumbers[layer.Number]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "number"), "duplicate"))
		}
		layerNumbers[layer.Number] = struct{}{}
		if _, ok := layerNames[layer.Name]; ok {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "duplicate"))
		}
		layerNames[layer.Name] = struct{}{}
		if !kicadfiles.IsValidBoardLayer(layer.Name) {
			errs = append(errs, fieldError(indexed("layers", i, "name"), "invalid"))
		}
		if strings.TrimSpace(layer.Kind) == "" {
			errs = append(errs, fieldError(indexed("layers", i, "kind"), "required"))
		}
	}
	netCodes := make(map[int]struct{}, len(board.Nets))
	netNames := make(map[string]struct{}, len(board.Nets))
	for i, net := range board.Nets {
		if net.Code < 0 {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "must be non-negative"))
		}
		if net.Code == 0 && net.Name != "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "must be empty for net 0"))
		}
		if net.Code > 0 && strings.TrimSpace(net.Name) == "" {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "required"))
		}
		if _, ok := netCodes[net.Code]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "code"), "duplicate"))
		}
		netCodes[net.Code] = struct{}{}
		if _, ok := netNames[net.Name]; ok {
			errs = append(errs, fieldError(indexed("nets", i, "name"), "duplicate"))
		}
		netNames[net.Name] = struct{}{}
	}
	return errs.Err()
}

func Write(w io.Writer, board PCBFile) error {
	if err := Validate(board); err != nil {
		return err
	}
	node, err := render(board)
	if err != nil {
		return err
	}
	return sexpr.Render(w, node)
}

func render(board PCBFile) (sexpr.List, error) {
	version, err := strconv.ParseInt(string(board.Version), 10, 64)
	if err != nil {
		return nil, err
	}
	nodes := []sexpr.Node{
		sexpr.A("kicad_pcb"),
		sexpr.L(sexpr.A("version"), sexpr.I(version)),
		sexpr.L(sexpr.A("generator"), sexpr.S(strings.TrimSpace(board.Generator))),
		sexpr.L(sexpr.A("general")),
		sexpr.L(sexpr.A("paper"), sexpr.S(strings.TrimSpace(board.Paper.Name))),
		renderLayers(board.Layers),
		renderSetup(board.Setup),
	}
	if title := renderTitleBlock(board.TitleBlock); len(title) > 1 {
		nodes = append(nodes, title)
	}
	for _, net := range sortedNets(board.Nets) {
		nodes = append(nodes, sexpr.L(sexpr.A("net"), sexpr.I(int64(net.Code)), sexpr.S(net.Name)))
	}
	return sexpr.L(nodes...), nil
}

func renderLayers(layers []LayerDefinition) sexpr.List {
	nodes := []sexpr.Node{sexpr.A("layers")}
	ordered := slices.Clone(layers)
	slices.SortFunc(ordered, func(a, b LayerDefinition) int { return cmp.Compare(a.Number, b.Number) })
	for _, layer := range ordered {
		nodes = append(nodes, sexpr.L(sexpr.I(int64(layer.Number)), sexpr.S(string(layer.Name)), sexpr.A(layer.Kind)))
	}
	return sexpr.L(nodes...)
}

func renderSetup(setup PCBSetup) sexpr.List {
	return sexpr.L(
		sexpr.A("setup"),
		sexpr.L(sexpr.A("stackup"), sexpr.L(sexpr.A("thickness"), sexpr.X(kicadfiles.ToMMString(setup.Stackup.Thickness)))),
		sexpr.L(sexpr.A("solder_mask_min_width"), sexpr.X(kicadfiles.ToMMString(setup.SolderMaskMinWidth))),
		sexpr.L(sexpr.A("pad_to_mask_clearance"), sexpr.X(kicadfiles.ToMMString(setup.PadToMaskClearance))),
	)
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

func sortedNets(nets []Net) []Net {
	ordered := slices.Clone(nets)
	if !hasNetZero(ordered) {
		ordered = append(ordered, Net{Code: 0, Name: ""})
	}
	slices.SortFunc(ordered, func(a, b Net) int { return cmp.Compare(a.Code, b.Code) })
	return ordered
}

func hasNetZero(nets []Net) bool {
	for _, net := range nets {
		if net.Code == 0 {
			return true
		}
	}
	return false
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "pcb", Field: field, Message: message}
}

func indexed(collection string, index int, field string) string {
	return collection + "[" + strconv.Itoa(index) + "]." + field
}
