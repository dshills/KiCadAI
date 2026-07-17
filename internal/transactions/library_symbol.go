package transactions

import (
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
)

func symbolRecordPins(record libraryresolver.SymbolRecord) ([]PinSpec, error) {
	return symbolRecordPinsForUnit(record, 0)
}

func symbolRecordPinsForUnit(record libraryresolver.SymbolRecord, unit int) ([]PinSpec, error) {
	targetUnit := unit
	if targetUnit <= 0 {
		targetUnit = 1
	}
	// KiCad serializes every distinct physical pin on each unit instance. A
	// repeated pin number across units denotes a shared/common pin instead, so
	// those symbols retain unit-local pin expansion.
	includeAllUnits := symbolHasDistinctUnitPins(record)
	seen := map[string]struct{}{}
	pins := make([]PinSpec, 0, len(record.Pins))
	for _, pin := range record.Pins {
		if pin.Unit != 0 && !includeAllUnits && pin.Unit != targetUnit {
			continue
		}
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

func symbolHasDistinctUnitPins(record libraryresolver.SymbolRecord) bool {
	unitsByNumber := map[string]map[int]struct{}{}
	unitCount := map[int]struct{}{}
	for _, pin := range record.Pins {
		if pin.Unit == 0 {
			continue
		}
		number := strings.TrimSpace(pin.Number)
		if number == "" {
			continue
		}
		if unitsByNumber[number] == nil {
			unitsByNumber[number] = map[int]struct{}{}
		}
		unitsByNumber[number][pin.Unit] = struct{}{}
		unitCount[pin.Unit] = struct{}{}
	}
	if len(unitCount) < 2 {
		return false
	}
	for _, units := range unitsByNumber {
		if len(units) > 1 {
			return false
		}
	}
	return true
}

func resolveSymbolPins(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string) ([]PinSpec, error) {
	return resolveSymbolPinsForUnit(pins, index, libraryID, 0)
}

func resolveSymbolPinsForUnit(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string, unit int) ([]PinSpec, error) {
	pins = append([]PinSpec(nil), pins...)
	if templatePins, ok := schematic.EmbeddedSymbolConnectionPinOffsets(libraryID); ok {
		templateByNumber := make(map[string]PinSpec, len(templatePins))
		for _, pin := range templatePins {
			templateByNumber[strings.TrimSpace(pin.Number)] = PinSpec{
				Number: pin.Number,
				XMM:    iuToMM(pin.Offset.X),
				YMM:    iuToMM(pin.Offset.Y),
			}
		}
		if len(pins) == 0 {
			resolved := make([]PinSpec, 0, len(templatePins))
			for _, pin := range templatePins {
				resolved = append(resolved, templateByNumber[strings.TrimSpace(pin.Number)])
			}
			return resolved, nil
		}
		resolved := make([]PinSpec, 0, len(pins))
		for _, pin := range pins {
			if pin.ExplicitOffset {
				resolved = append(resolved, pin)
				continue
			}
			if templatePin, ok := templateByNumber[strings.TrimSpace(pin.Number)]; ok {
				pin.XMM = templatePin.XMM
				pin.YMM = templatePin.YMM
			}
			resolved = append(resolved, pin)
		}
		return resolved, nil
	}
	if index == nil {
		return pins, nil
	}
	record, ok := libraryresolver.ResolveSymbol(*index, libraryID)
	if !ok {
		return pins, nil
	}
	return mergeSymbolRecordPinsForUnit(pins, record, unit)
}

func resolvePreferredSymbolPinsForUnit(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string, unit int, preferResolver bool) ([]PinSpec, error) {
	if !preferResolver || index == nil {
		return resolveSymbolPinsForUnit(pins, index, libraryID, unit)
	}
	record, ok := libraryresolver.ResolveSymbol(*index, libraryID)
	if !ok || strings.TrimSpace(record.Raw) == "" {
		return resolveSymbolPinsForUnit(pins, index, libraryID, unit)
	}
	return mergeSymbolRecordPinsForUnit(append([]PinSpec(nil), pins...), record, unit)
}

func mergeSymbolRecordPinsForUnit(pins []PinSpec, record libraryresolver.SymbolRecord, unit int) ([]PinSpec, error) {
	pins = append([]PinSpec(nil), pins...)
	resolved, err := symbolRecordPinsForUnit(record, unit)
	if err != nil {
		if len(pins) > 0 {
			for _, pin := range pins {
				if !pin.ExplicitOffset {
					return nil, err
				}
			}
			return pins, nil
		}
		return nil, err
	}
	type resolvedPinMatch struct {
		pin       PinSpec
		canonical string
	}
	resolvedByNumber := make(map[string]resolvedPinMatch, len(resolved))
	for _, pin := range resolved {
		canonical := strings.TrimSpace(pin.Number)
		for _, member := range libraryresolver.GroupedPinMembers(canonical) {
			memberPin := pin
			memberPin.Number = member
			resolvedByNumber[member] = resolvedPinMatch{pin: memberPin, canonical: canonical}
		}
	}
	if len(pins) > 0 {
		provided := make(map[string]struct{}, len(pins))
		for i := range pins {
			number := strings.TrimSpace(pins[i].Number)
			if pins[i].ExplicitOffset {
				provided[number] = struct{}{}
				continue
			}
			if match, ok := resolvedByNumber[number]; ok {
				pins[i].XMM = match.pin.XMM
				pins[i].YMM = match.pin.YMM
				provided[number] = struct{}{}
				provided[match.canonical] = struct{}{}
			} else {
				return nil, fmt.Errorf("symbol %s has no resolver pin %s for unit %d", record.LibraryID, number, unit)
			}
		}
		merged := make([]PinSpec, 0, len(pins)+len(resolved))
		merged = append(merged, pins...)
		for _, pin := range resolved {
			number := strings.TrimSpace(pin.Number)
			if _, exists := provided[number]; !exists {
				merged = append(merged, pin)
			}
		}
		return merged, nil
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("symbol library record found but has no usable pins: %s", record.LibraryID)
	}
	return resolved, nil
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / float64(kicadfiles.MM(1))
}
