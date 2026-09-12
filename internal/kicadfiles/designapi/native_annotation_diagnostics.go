package designapi

import (
	"fmt"
	"sort"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/schematiclayout"
)

// NativeAnnotationCandidate describes one geometrically valid label placement.
type NativeAnnotationCandidate struct {
	At       kicadfiles.Point     `json:"at"`
	Box      schematiclayout.Rect `json:"box"`
	Rotation kicadfiles.Angle     `json:"rotation"`
}

// NativeAnnotationLabelDiagnostic preserves the solver's constrained-first order.
type NativeAnnotationLabelDiagnostic struct {
	UUID       kicadfiles.UUID             `json:"uuid"`
	Text       string                      `json:"text"`
	Origin     kicadfiles.Point            `json:"origin"`
	Candidates []NativeAnnotationCandidate `json:"candidates"`
}

// NativeAnnotationDiagnostics inspects an isolated projection. It neither writes
// a project nor changes the routing state used by subsequent Design/Write calls.
func (builder *Builder) NativeAnnotationDiagnostics() ([]NativeAnnotationLabelDiagnostic, error) {
	if builder == nil || builder.design.Schematic == nil {
		return nil, fmt.Errorf("native annotation diagnostics require a schematic")
	}
	if builder.nativeSchematicProfile != schematiclayout.NativeAnnotationV2 {
		return nil, fmt.Errorf("native annotation diagnostics require annotation-v2")
	}
	original := builder.design.Schematic
	originalIDs := builder.finalRouteLabelUUIDs
	builder.design.Schematic = cloneDesign(builder.design).Schematic
	builder.finalRouteLabelUUIDs = make(map[kicadfiles.UUID]struct{}, len(originalIDs))
	for id := range originalIDs {
		builder.finalRouteLabelUUIDs[id] = struct{}{}
	}
	defer func() {
		builder.design.Schematic = original
		builder.finalRouteLabelUUIDs = originalIDs
	}()
	builder.finalizeSchematicRouteLabels()
	bodies := builder.nativeAnnotationBodies()
	labels := make([]NativeAnnotationLabelDiagnostic, 0, len(builder.design.Schematic.Labels))
	for _, label := range builder.design.Schematic.Labels {
		d := NativeAnnotationLabelDiagnostic{UUID: label.UUID, Text: label.Text, Origin: label.Position}
		for _, candidate := range builder.nativeLabelCandidates(label, bodies) {
			d.Candidates = append(d.Candidates, NativeAnnotationCandidate{At: candidate.at, Box: candidate.box, Rotation: candidate.options.Rotation})
		}
		labels = append(labels, d)
	}
	sort.SliceStable(labels, func(i, j int) bool {
		if len(labels[i].Candidates) != len(labels[j].Candidates) {
			return len(labels[i].Candidates) < len(labels[j].Candidates)
		}
		return string(labels[i].UUID) < string(labels[j].UUID)
	})
	if len(builder.finalRouteLabelUUIDs) != len(builder.pendingRouteLabels) {
		return labels, fmt.Errorf("native annotation profile lost a required routed-net label")
	}
	return labels, builder.finalizeNativeAnnotations()
}
