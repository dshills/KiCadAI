package pcb

import (
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

func rawRootToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '(' {
		return ""
	}
	rest := strings.TrimLeft(raw[1:], " \t\r\n")
	end := strings.IndexAny(rest, " \t\r\n()")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func knownPCBTopLevelNode(token string) bool {
	switch token {
	case "version", "generator", "generator_version", "general", "paper", "title_block", "layers", "setup", "net", "net_class", "footprint", "gr_line", "gr_rect", "gr_circle", "gr_arc", "gr_poly", "gr_curve", "gr_text", "segment", "arc", "via", "zone", "dimension", "embedded_fonts", "teardrops", "group", "image", "table", "target", "embedded_files", "component_classes":
		return true
	default:
		return false
	}
}

func isModeledSingleInstancePCBNode(token string, board PCBFile) bool {
	switch token {
	case "version", "generator", "generator_version", "general", "paper", "title_block", "layers", "setup":
		return true
	case "embedded_fonts":
		return board.EmbeddedFonts != nil
	default:
		return false
	}
}

func insertPreservedNodes(nodes []sexpr.Node, preserved []PreservedNode) []sexpr.Node {
	childrenByAnchor := map[string][]int{}
	roots := make([]int, 0, len(preserved))
	for i, preservedNode := range preserved {
		after := strings.TrimSpace(preservedNode.After)
		if after == "" {
			roots = append(roots, i)
			continue
		}
		childrenByAnchor[after] = append(childrenByAnchor[after], i)
	}
	out := make([]sexpr.Node, 0, len(nodes)+len(preserved))
	emitted := make([]bool, len(preserved))
	var appendPreserved func(index int)
	appendPreserved = func(index int) {
		if emitted[index] {
			return
		}
		emitted[index] = true
		raw := preserved[index].Raw
		out = append(out, sexpr.R(raw))
		if token := rawRootToken(raw); token != "" {
			for _, childIndex := range childrenByAnchor[token] {
				appendPreserved(childIndex)
			}
		}
	}
	appendChildren := func(anchor string) {
		for _, index := range childrenByAnchor[anchor] {
			appendPreserved(index)
		}
	}
	for _, node := range nodes {
		out = append(out, node)
		if token := nodeRootToken(node); token != "" {
			appendChildren(token)
		}
	}
	for _, index := range roots {
		appendPreserved(index)
	}
	for i := range preserved {
		if !emitted[i] {
			appendPreserved(i)
		}
	}
	return out
}

func validatePreservedNodeAnchorGraph(preserved []PreservedNode) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	indexByToken := map[string]int{}
	for i, preservedNode := range preserved {
		if token := rawRootToken(preservedNode.Raw); token != "" {
			if _, exists := indexByToken[token]; !exists {
				indexByToken[token] = i
			}
		}
	}
	for i := range preserved {
		seen := map[int]struct{}{}
		for current := i; ; {
			after := strings.TrimSpace(preserved[current].After)
			if after == "" {
				break
			}
			next, ok := indexByToken[after]
			if !ok {
				break
			}
			if _, ok := seen[next]; ok {
				errs = append(errs, fieldError(indexed("preserved", i, "after"), "cyclic preserved node anchor"))
				break
			}
			seen[next] = struct{}{}
			current = next
		}
	}
	return errs
}

func nodeRootToken(node sexpr.Node) string {
	list, ok := node.(sexpr.List)
	if !ok || len(list) == 0 {
		return ""
	}
	atom, ok := list[0].(sexpr.Atom)
	if !ok {
		return ""
	}
	return string(atom)
}
