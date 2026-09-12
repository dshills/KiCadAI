package schematic

import "kicadai/internal/kicadfiles/sexpr"

// readNativeV10Text models only the exact supported default-font form. Unknown
// effects, attributes, duplicate nodes, fonts, or simulation exclusion remain
// raw preservation content; no field is silently discarded.
func readNativeV10Text(node sexpr.ParsedNode) (Text, bool) {
	if len(node.Children) < 2 {
		return Text{}, false
	}
	allowed := map[string]bool{"exclude_from_sim": true, "at": true, "effects": true, "uuid": true, "locked": true}
	seen := map[string]bool{}
	for _, child := range node.Children[2:] {
		if !allowed[child.Head()] || seen[child.Head()] {
			return Text{}, false
		}
		seen[child.Head()] = true
	}
	excluded, ok := node.Child("exclude_from_sim")
	if !ok || len(excluded.Children) != 2 || excluded.ListValue(1) != "no" {
		return Text{}, false
	}
	at, aok := node.Child("at")
	if !aok || len(at.Children) != 4 {
		return Text{}, false
	}
	position, rotation, aok := readAt(node)
	if !aok {
		return Text{}, false
	}
	effects, eok := node.Child("effects")
	if !eok || len(effects.Children) != 2 {
		return Text{}, false
	}
	font, fok := effects.Child("font")
	if !fok || len(font.Children) != 2 {
		return Text{}, false
	}
	size, sok := font.Child("size")
	if !sok || len(size.Children) != 3 {
		return Text{}, false
	}
	x, xok := size.FloatValue(1)
	y, yok := size.FloatValue(2)
	if !xok || !yok || x != 1.27 || y != 1.27 {
		return Text{}, false
	}
	uuid, uok := node.Child("uuid")
	if !uok || len(uuid.Children) != 2 {
		return Text{}, false
	}
	locked, lok := node.Child("locked")
	if lok && (len(locked.Children) != 2 || locked.ListValue(1) != "yes") {
		return Text{}, false
	}
	return Text{NativeV10: true, UUID: readUUID(node), Value: node.ListValue(1), Position: position, Rotation: rotation, Locked: lok}, true
}
