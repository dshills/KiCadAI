package libraryresolver

import "strings"

// GroupedPinMembers expands the bracketed pin-number notation used by KiCad
// symbols whose single graphical pin represents multiple physical pins. Plain
// pin numbers are returned as a one-element slice.
func GroupedPinMembers(pin string) []string {
	pin = strings.TrimSpace(pin)
	if len(pin) < 3 || pin[0] != '[' || pin[len(pin)-1] != ']' {
		if pin == "" {
			return nil
		}
		return []string{pin}
	}
	parts := strings.Split(pin[1:len(pin)-1], ",")
	members := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		members = append(members, part)
	}
	return members
}

// CanonicalSymbolPinNumber returns the resolver pin number KiCad serializes
// for a logical or physical member pin. Unit-specific definitions take
// precedence over common unit-zero definitions.
func CanonicalSymbolPinNumber(record SymbolRecord, unit int, number string) (string, bool) {
	number = strings.TrimSpace(number)
	if number == "" {
		return "", false
	}
	for _, candidateUnit := range []int{unit, 0} {
		for _, pin := range record.Pins {
			canonical := strings.TrimSpace(pin.Number)
			if canonical == "" || pin.Unit != candidateUnit {
				continue
			}
			for _, member := range GroupedPinMembers(canonical) {
				if member == number || canonical == number {
					return canonical, true
				}
			}
		}
	}
	return "", false
}
