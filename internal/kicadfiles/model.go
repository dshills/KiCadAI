package kicadfiles

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type KiCadFormatVersion string

const (
	KiCadFormatV20230121 KiCadFormatVersion = "20230121"

	KiCadPCBFormatV20260206 KiCadFormatVersion = "20260206"

	KiCadSchematicFormatV20260306 KiCadFormatVersion = "20260306"
	KiCadSchematicFormatV20260512 KiCadFormatVersion = "20260512"

	// KiCadFormatV20260306 is retained for callers created before schematic
	// and PCB format constants were split by file type.
	KiCadFormatV20260306 KiCadFormatVersion = KiCadSchematicFormatV20260306
)

const KiCadSchematicFormatWithGeneratorVersion int64 = 20231120

type UUID string
type IU int64
type Angle float64

type Point struct {
	X IU
	Y IU
}

// SchematicLibraryPoint converts a KiCad symbol-library coordinate into the
// schematic coordinate frame used by wires and symbol instances.
func SchematicLibraryPoint(point Point) Point {
	point.Y = -point.Y
	return point
}

// SchematicLibraryBounds converts an axis-aligned library bounds pair into
// schematic coordinates while preserving min/max ordering.
func SchematicLibraryBounds(minPoint, maxPoint Point) (Point, Point) {
	return Point{X: minPoint.X, Y: -maxPoint.Y}, Point{X: maxPoint.X, Y: -minPoint.Y}
}

type Paper struct {
	Name   string
	Width  IU
	Height IU
}

type TitleBlock struct {
	Title    string
	Date     string
	Revision string
	Company  string
	Comments []string
}

type BoardLayer string

const (
	LayerFCu      BoardLayer = "F.Cu"
	LayerBCu      BoardLayer = "B.Cu"
	LayerFAdhes   BoardLayer = "F.Adhes"
	LayerBAdhes   BoardLayer = "B.Adhes"
	LayerFPaste   BoardLayer = "F.Paste"
	LayerBPaste   BoardLayer = "B.Paste"
	LayerFSilkS   BoardLayer = "F.SilkS"
	LayerBSilkS   BoardLayer = "B.SilkS"
	LayerFMask    BoardLayer = "F.Mask"
	LayerBMask    BoardLayer = "B.Mask"
	LayerFCrtYd   BoardLayer = "F.CrtYd"
	LayerBCrtYd   BoardLayer = "B.CrtYd"
	LayerFFab     BoardLayer = "F.Fab"
	LayerBFab     BoardLayer = "B.Fab"
	LayerEdge     BoardLayer = "Edge.Cuts"
	LayerMargin   BoardLayer = "Margin"
	LayerDwgs     BoardLayer = "Dwgs.User"
	LayerCmts     BoardLayer = "Cmts.User"
	LayerEco1     BoardLayer = "Eco1.User"
	LayerEco2     BoardLayer = "Eco2.User"
	LayerUserDwgs BoardLayer = "User.Drawings"
	LayerUserCmts BoardLayer = "User.Comments"
	LayerAllCu    BoardLayer = "*.Cu"
	LayerAllMask  BoardLayer = "*.Mask"
	LayerAll      BoardLayer = "All"
)

const (
	iuPerMM  = 1_000_000
	mmPerMil = 0.0254
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var internalCopperLayerPattern = regexp.MustCompile(`^In([1-9]|[12][0-9]|30)\.Cu$`)

func MM(value float64) IU {
	return IU(math.Round(value * iuPerMM))
}

func Mil(value float64) IU {
	return MM(value * mmPerMil)
}

func ToMMString(value IU) string {
	negative := value < 0
	var absolute uint64
	if negative {
		absolute = uint64(-(int64(value) + 1)) + 1
	} else {
		absolute = uint64(value)
	}

	digits := strconv.AppendUint(make([]byte, 0, 20), absolute, 10)
	if len(digits) < 7 {
		padded := make([]byte, 7)
		copy(padded[7-len(digits):], digits)
		for i := 0; i < 7-len(digits); i++ {
			padded[i] = '0'
		}
		digits = padded
	}
	point := len(digits) - 6
	out := make([]byte, 0, len(digits)+2)
	if negative {
		out = append(out, '-')
	}
	out = append(out, digits[:point]...)
	out = append(out, '.')
	fraction := digits[point:]
	trimTo := len(fraction)
	for trimTo > 1 && fraction[trimTo-1] == '0' {
		trimTo--
	}
	out = append(out, fraction[:trimTo]...)
	return string(out)
}

type IDGenerator interface {
	New(scope string, parts ...string) UUID
}

type DeterministicIDGenerator struct {
	namespace [16]byte
	seed      string
}

func NewDeterministicIDGenerator(designID UUID, seed string) (DeterministicIDGenerator, error) {
	namespace, err := parseUUIDBytes(designID)
	if err != nil {
		return DeterministicIDGenerator{}, err
	}
	return DeterministicIDGenerator{namespace: namespace, seed: normalizeNFC(seed)}, nil
}

func (generator DeterministicIDGenerator) New(scope string, parts ...string) UUID {
	name := appendLengthPrefixed(nil, generator.seed)
	name = append(name, ':')
	name = appendPathComponent(name, scope)
	for _, part := range parts {
		name = append(name, ':')
		name = appendPathComponent(name, part)
	}
	return uuidV5(generator.namespace, name)
}

func uuidV5(namespace [16]byte, name []byte) UUID {
	hash := sha1.New()
	hash.Write(namespace[:])
	hash.Write(name)
	sum := hash.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	return formatUUIDBytes(sum)
}

func (uuid UUID) Valid() bool {
	return uuidPattern.MatchString(string(uuid))
}

func IsValidBoardLayer(layer BoardLayer) bool {
	switch layer {
	case LayerFCu, LayerBCu,
		LayerFAdhes, LayerBAdhes,
		LayerFPaste, LayerBPaste,
		LayerFSilkS, LayerBSilkS,
		LayerFMask, LayerBMask,
		LayerFCrtYd, LayerBCrtYd,
		LayerFFab, LayerBFab,
		LayerEdge, LayerMargin,
		LayerDwgs, LayerCmts, LayerEco1, LayerEco2,
		LayerUserDwgs, LayerUserCmts,
		LayerAllCu, LayerAllMask, LayerAll:
		return true
	default:
		name := string(layer)
		if internalCopperLayerPattern.MatchString(name) {
			return true
		}
		if strings.HasPrefix(name, "User.") {
			number, err := strconv.Atoi(strings.TrimPrefix(name, "User."))
			return err == nil && number >= 1 && number <= 45
		}
		return false
	}
}

type ValidationError struct {
	File    string
	Section string
	Field   string
	Message string
}

func (err ValidationError) Error() string {
	location := strings.Trim(strings.Join([]string{err.File, err.Section, err.Field}, "."), ".")
	if location == "" {
		return err.Message
	}
	return location + ": " + err.Message
}

type ValidationErrors []ValidationError

func (errs ValidationErrors) Error() string {
	if len(errs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "; ")
}

func (errs ValidationErrors) Err() error {
	if len(errs) == 0 {
		return nil
	}
	return errs
}

var ErrInvalidUUID = errors.New("invalid uuid")

func parseUUIDBytes(uuid UUID) ([16]byte, error) {
	var out [16]byte
	if !uuid.Valid() {
		return out, fmt.Errorf("%w: %s", ErrInvalidUUID, uuid)
	}
	compact := strings.ReplaceAll(string(uuid), "-", "")
	decoded, err := hex.DecodeString(compact)
	if err != nil || len(decoded) != len(out) {
		return out, fmt.Errorf("%w: %s", ErrInvalidUUID, uuid)
	}
	copy(out[:], decoded)
	return out, nil
}

func formatUUIDBytes(bytes []byte) UUID {
	hexed := hex.EncodeToString(bytes)
	return UUID(hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32])
}

func appendLengthPrefixed(out []byte, value string) []byte {
	out = strconv.AppendInt(out, int64(len(value)), 10)
	out = append(out, ':')
	out = append(out, value...)
	return out
}

func appendPathComponent(out []byte, value string) []byte {
	return appendLengthPrefixed(out, normalizeNFC(value))
}

func normalizeNFC(value string) string {
	return norm.NFC.String(value)
}
