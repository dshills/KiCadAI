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
	seen := map[string]struct{}{}
	pins := make([]PinSpec, 0, len(record.Pins))
	for _, pin := range record.Pins {
		if pin.Unit != 0 && pin.Unit != targetUnit {
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

func resolveSymbolPins(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string) ([]PinSpec, error) {
	return resolveSymbolPinsForUnit(pins, index, libraryID, 0)
}

func resolveSymbolPinsForUnit(pins []PinSpec, index *libraryresolver.LibraryIndex, libraryID string, unit int) ([]PinSpec, error) {
	pins = append([]PinSpec(nil), pins...)
	if templatePins, ok := schematic.EmbeddedSymbolPinOffsets(libraryID); ok {
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
	resolvedByNumber := make(map[string]PinSpec, len(resolved))
	for _, pin := range resolved {
		resolvedByNumber[strings.TrimSpace(pin.Number)] = pin
	}
	if len(pins) > 0 {
		for i := range pins {
			if pins[i].ExplicitOffset {
				continue
			}
			if pin, ok := resolvedByNumber[strings.TrimSpace(pins[i].Number)]; ok {
				pins[i].XMM = pin.XMM
				pins[i].YMM = pin.YMM
			} else {
				return nil, fmt.Errorf("symbol %s has no resolver pin %s for unit %d", libraryID, strings.TrimSpace(pins[i].Number), unit)
			}
		}
		return pins, nil
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("symbol library record found but has no usable pins: %s", libraryID)
	}
	return resolved, nil
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / float64(kicadfiles.MM(1))
}
