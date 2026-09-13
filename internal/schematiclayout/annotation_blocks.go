package schematiclayout

// NativeAnnotationBlock is drawing-only explanatory text associated with exact
// schematic references. It never defines or aliases an electrical net.
type NativeAnnotationBlock struct {
	ID         string   `json:"id"`
	References []string `json:"references"`
	Lines      []string `json:"lines"`
}

func CloneNativeAnnotationBlocks(blocks []NativeAnnotationBlock) []NativeAnnotationBlock {
	clone := append([]NativeAnnotationBlock(nil), blocks...)
	for i := range clone {
		clone[i].References = append([]string(nil), blocks[i].References...)
		clone[i].Lines = append([]string(nil), blocks[i].Lines...)
	}
	return clone
}
