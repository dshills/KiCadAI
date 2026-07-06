package blocks

import "kicadai/internal/transactions"

func twoTerminalHorizontalPins() []transactions.PinSpec {
	// Generic schematic anchors for two-terminal symbols that do not yet have
	// symbol-specific KiCad 10 pin geometry.
	return []transactions.PinSpec{
		{Number: "1", XMM: -5.08, YMM: 0},
		{Number: "2", XMM: 5.08, YMM: 0},
	}
}

func twoTerminalVerticalPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: 0, YMM: -5.08},
		{Number: "2", XMM: 0, YMM: 5.08},
	}
}
