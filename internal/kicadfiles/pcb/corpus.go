package pcb

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CorpusReport struct {
	Files               int
	ObjectCount         map[string]int
	TopLevelObjects     map[string]int
	FootprintChildTypes map[string]int
	PadTypes            map[string]int
	PadShapes           map[string]int
	LayerUsage          map[string]int
	ZoneLayers          map[string]int
	// ScalarCount records enum-like late scalars only; numeric coordinates,
	// UUIDs, and other high-cardinality values are intentionally filtered out.
	ScalarCount        map[string]int
	SupportedObjects   map[string]int
	PreservationOnly   map[string]int
	UnsupportedObjects map[string]int
}

func ScanCorpus(root string) (CorpusReport, error) {
	report := newCorpusReport()
	var scanErrs []error
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			scanErrs = append(scanErrs, err)
			return nil
		}
		if entry.IsDir() || !isCorpusPCBFile(entry.Name()) {
			return nil
		}
		fileReport, err := scanCorpusFile(path)
		if err != nil {
			scanErrs = append(scanErrs, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		fileReport.Files = 1
		report.merge(fileReport)
		return nil
	})
	if err != nil {
		scanErrs = append(scanErrs, err)
	}
	return report, errors.Join(scanErrs...)
}

func scanCorpusFile(path string) (CorpusReport, error) {
	report := newCorpusReport()
	file, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer file.Close()
	return report, scanPCBFileInto(file, &report)
}

func isCorpusPCBFile(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".kicad_pcb") || strings.HasSuffix(name, ".kicad_mod")
}

func scanPCBObjects(r io.Reader) (map[string]int, error) {
	report, err := scanPCBFile(r)
	return report.ObjectCount, err
}

func scanPCBFile(r io.Reader) (CorpusReport, error) {
	report := newCorpusReport()
	err := scanPCBFileInto(r, &report)
	return report, err
}

func scanPCBFileInto(r io.Reader, report *CorpusReport) error {
	reader := bufio.NewReader(r)
	var stack []corpusFrame
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			if len(stack) > 0 {
				return io.ErrUnexpectedEOF
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch b {
		case '"':
			scalar, err := readQuotedScalarAfterOpeningQuote(reader)
			if err != nil {
				return err
			}
			report.recordScalar(stack, scalar)
		case ';':
			if err := skipLineComment(reader); err != nil {
				return err
			}
		case '(':
			token, err := readObjectToken(reader)
			if err != nil {
				return err
			}
			parent := currentFrame(stack)
			scalars, err := readImmediateScalars(reader, shouldCollectImmediateScalars(token, parent, stack), report, stack)
			if err != nil {
				return err
			}
			report.record(token, parent, stack, scalars)
			if token != "" {
				report.ObjectCount[token]++
			}
			stack = append(stack, corpusFrame{token: token})
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			if !isCorpusSpace(b) {
				scalar, err := readBareScalar(reader, b)
				if err != nil {
					return err
				}
				report.recordScalar(stack, scalar)
			}
		}
	}
}

type corpusFrame struct {
	token string
}

func newCorpusReport() CorpusReport {
	return CorpusReport{
		ObjectCount:         map[string]int{},
		TopLevelObjects:     map[string]int{},
		FootprintChildTypes: map[string]int{},
		PadTypes:            map[string]int{},
		PadShapes:           map[string]int{},
		LayerUsage:          map[string]int{},
		ZoneLayers:          map[string]int{},
		ScalarCount:         map[string]int{},
		SupportedObjects:    map[string]int{},
		PreservationOnly:    map[string]int{},
		UnsupportedObjects:  map[string]int{},
	}
}

func (report *CorpusReport) merge(other CorpusReport) {
	report.Files += other.Files
	mergeCounts(report.ObjectCount, other.ObjectCount)
	mergeCounts(report.TopLevelObjects, other.TopLevelObjects)
	mergeCounts(report.FootprintChildTypes, other.FootprintChildTypes)
	mergeCounts(report.PadTypes, other.PadTypes)
	mergeCounts(report.PadShapes, other.PadShapes)
	mergeCounts(report.LayerUsage, other.LayerUsage)
	mergeCounts(report.ZoneLayers, other.ZoneLayers)
	mergeCounts(report.ScalarCount, other.ScalarCount)
	mergeCounts(report.SupportedObjects, other.SupportedObjects)
	mergeCounts(report.PreservationOnly, other.PreservationOnly)
	mergeCounts(report.UnsupportedObjects, other.UnsupportedObjects)
}

func mergeCounts(dst, src map[string]int) {
	for key, count := range src {
		dst[key] += count
	}
}

func (report *CorpusReport) record(token string, parent corpusFrame, stack []corpusFrame, scalars []string) {
	if token == "" {
		if parent.token == "layers" && len(scalars) >= 2 {
			report.LayerUsage[scalars[1]]++
		}
		return
	}
	if parent.token == "kicad_pcb" {
		report.TopLevelObjects[token]++
	}
	if parent.token == "footprint" {
		report.FootprintChildTypes[token]++
	}
	if token == "pad" {
		if len(scalars) >= 2 {
			report.PadTypes[scalars[1]]++
		}
		if len(scalars) >= 3 {
			report.PadShapes[scalars[2]]++
		}
	}
	if token == "layer" && len(scalars) >= 1 {
		report.LayerUsage[scalars[0]]++
		if hasAncestor(stack, "zone") {
			report.ZoneLayers[scalars[0]]++
		}
	}
	if token == "layers" {
		for _, layer := range scalars {
			report.LayerUsage[layer]++
		}
	}
	switch objectSupport(token) {
	case corpusObjectSupported:
		report.SupportedObjects[token]++
	case corpusObjectPreservationOnly:
		report.PreservationOnly[token]++
	case corpusObjectUnsupported:
		report.UnsupportedObjects[token]++
	}
}

func (report *CorpusReport) recordScalar(stack []corpusFrame, scalar string) {
	if len(stack) == 0 || !isCorpusScalarToken(scalar) {
		return
	}
	report.ScalarCount[scalar]++
}

type corpusObjectSupport int

const (
	corpusObjectSupported corpusObjectSupport = iota
	corpusObjectPreservationOnly
	corpusObjectUnsupported
)

func objectSupport(token string) corpusObjectSupport {
	switch token {
	case "kicad_pcb", "version", "generator", "generator_version", "general", "paper", "title_block", "layers", "setup", "pcbplotparams",
		"uuid", "property", "net", "footprint", "fp_text", "fp_line", "fp_rect", "fp_circle", "fp_arc", "fp_poly", "pad", "model",
		"gr_line", "gr_rect", "gr_circle", "gr_arc", "gr_poly", "gr_text", "segment", "arc", "via", "zone", "polygon", "filled_polygon", "pts", "xy", "layer",
		"dimension", "start", "end", "width", "at", "size", "drill", "effects", "font", "stroke", "fill", "locked", "placed", "descr", "tags", "attr":
		return corpusObjectSupported
	case "embedded_fonts", "teardrops", "group", "image", "table", "target", "embedded_files", "component_classes":
		return corpusObjectPreservationOnly
	default:
		return corpusObjectUnsupported
	}
}

func currentFrame(stack []corpusFrame) corpusFrame {
	if len(stack) == 0 {
		return corpusFrame{}
	}
	return stack[len(stack)-1]
}

func shouldCollectImmediateScalars(token string, parent corpusFrame, stack []corpusFrame) bool {
	switch token {
	case "pad", "layer", "layers":
		return true
	case "":
		return parent.token == "layers"
	default:
		return false
	}
}

func hasAncestor(stack []corpusFrame, token string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].token == token {
			return true
		}
	}
	return false
}

func skipLineComment(reader *bufio.Reader) error {
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if b == '\n' {
			return nil
		}
	}
}

func readImmediateScalars(reader *bufio.Reader, collect bool, report *CorpusReport, stack []corpusFrame) ([]string, error) {
	var scalars []string
	for {
		next, err := reader.Peek(1)
		if err == io.EOF {
			return scalars, nil
		}
		if err != nil {
			return nil, err
		}
		b := next[0]
		if isCorpusSpace(b) {
			if _, err := reader.ReadByte(); err != nil {
				return nil, err
			}
			continue
		}
		switch b {
		case '(':
			return scalars, nil
		case ')':
			return scalars, nil
		case '"':
			if _, err := reader.ReadByte(); err != nil {
				return nil, err
			}
			value, err := readQuotedScalarAfterOpeningQuote(reader)
			if err != nil {
				return nil, err
			}
			report.recordScalar(stack, value)
			if collect {
				scalars = append(scalars, value)
			}
		case ';':
			if _, err := reader.ReadByte(); err != nil {
				return nil, err
			}
			if err := skipLineComment(reader); err != nil {
				return nil, err
			}
		default:
			if _, err := reader.ReadByte(); err != nil {
				return nil, err
			}
			value, err := readBareScalar(reader, b)
			if err != nil {
				return nil, err
			}
			report.recordScalar(stack, value)
			if collect && value != "" {
				scalars = append(scalars, value)
			}
		}
	}
}

// readQuotedScalarAfterOpeningQuote reads until the closing quote. Callers must
// consume the opening quote before calling it.
func readQuotedScalarAfterOpeningQuote(reader *bufio.Reader) (string, error) {
	var value strings.Builder
	escaped := false
	for {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return value.String(), io.ErrUnexpectedEOF
		}
		if err != nil {
			return "", err
		}
		if escaped {
			escaped = false
			switch b {
			case 'n':
				value.WriteByte('\n')
			case 'r':
				value.WriteByte('\r')
			case 't':
				value.WriteByte('\t')
			case '\\', '"':
				value.WriteByte(b)
			default:
				value.WriteByte(b)
			}
			continue
		}
		switch b {
		case '\\':
			escaped = true
		case '"':
			return value.String(), nil
		default:
			value.WriteByte(b)
		}
	}
}

func readBareScalar(reader *bufio.Reader, first byte) (string, error) {
	var value strings.Builder
	value.WriteByte(first)
	for {
		next, err := reader.Peek(1)
		if err == io.EOF {
			return value.String(), nil
		}
		if err != nil {
			return "", err
		}
		b := next[0]
		if isCorpusSpace(b) || b == '(' || b == ')' || b == ';' {
			return value.String(), nil
		}
		if _, err := reader.ReadByte(); err != nil {
			return "", err
		}
		value.WriteByte(b)
	}
}

func readObjectToken(reader *bufio.Reader) (string, error) {
	var token strings.Builder
	for {
		next, err := reader.Peek(1)
		if err == io.EOF {
			return token.String(), nil
		}
		if err != nil {
			return "", err
		}
		b := next[0]
		if token.Len() == 0 {
			if isCorpusSpace(b) {
				if _, err := reader.ReadByte(); err != nil {
					return "", err
				}
				continue
			}
			if !isObjectTokenStart(b) {
				return "", nil
			}
			if _, err := reader.ReadByte(); err != nil {
				return "", err
			}
			token.WriteByte(b)
			continue
		}
		if !isObjectTokenPart(b) {
			return token.String(), nil
		}
		if _, err := reader.ReadByte(); err != nil {
			return "", err
		}
		token.WriteByte(b)
	}
}

func isObjectTokenStart(b byte) bool {
	return b == '_' || b == '*' || b == '+' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isObjectTokenPart(b byte) bool {
	return isObjectTokenStart(b) || b >= '0' && b <= '9' || b == '-' || b == '.' || b == ':' || b == '/'
}

func isCorpusScalarToken(value string) bool {
	if value == "" || len(value) > 64 || isUUIDLikeScalar(value) {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if r > 127 || !isObjectTokenStart(byte(r)) {
				return false
			}
			continue
		}
		if r > 127 || !isObjectTokenPart(byte(r)) {
			return false
		}
	}
	return true
}

func isUUIDLikeScalar(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
				return false
			}
		}
	}
	return true
}

func isCorpusSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}
