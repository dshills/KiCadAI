package transactions

import (
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
)

const internalUnitsPerMM = 1_000_000

func symbolRecordPins(record libraryresolver.SymbolRecord) ([]PinSpec, error) {
	if len(record.Units) > 1 {
		return nil, fmt.Errorf("symbol %s has multiple units; resolver-backed multi-unit placement is not implemented", record.LibraryID)
	}
	seen := map[string]struct{}{}
	pins := make([]PinSpec, 0, len(record.Pins))
	for _, pin := range record.Pins {
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			continue
		}
		if _, ok := seen[number]; ok {
			return nil, fmt.Errorf("symbol %s has duplicate pin number %s; stacked pin placement is not implemented", record.LibraryID, number)
		}
		seen[number] = struct{}{}
		pins = append(pins, PinSpec{
			Number: number,
			XMM:    iuToMM(pin.Position.X),
			YMM:    iuToMM(pin.Position.Y),
		})
	}
	if len(pins) == 0 {
		return nil, fmt.Errorf("symbol %s has no usable electrical pins", record.LibraryID)
	}
	return pins, nil
}

func resolveSymbolPins(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string) ([]PinSpec, error) {
	if len(pins) > 0 || index == nil {
		return pins, nil
	}
	record, ok := libraryresolver.ResolveSymbol(*index, libraryID)
	if !ok {
		return nil, fmt.Errorf("symbol library record not found: %s", libraryID)
	}
	return symbolRecordPins(record)
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / internalUnitsPerMM
}
