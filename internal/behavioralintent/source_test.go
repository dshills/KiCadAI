package behavioralintent

import "testing"

func TestPrepareSourceOwnsStableStatementBoundaries(t *testing.T) {
	source := PrepareSource("Use a 3.3 V supply. Gain must be 10.5; keep the output safe.\nExpose I2C?")
	if len(source.Statements) != 4 {
		t.Fatalf("statements = %#v", source.Statements)
	}
	if source.Statements[0].Text != "Use a 3.3 V supply." || source.Statements[0].ID != "statement_001" {
		t.Fatalf("first statement = %#v", source.Statements[0])
	}
	if source.SHA256 == "" || source.ByteLength == 0 {
		t.Fatalf("source evidence = %#v", source)
	}
	if repeated := PrepareSource("Use a 3.3 V supply. Gain must be 10.5; keep the output safe.\nExpose I2C?"); repeated.SHA256 != source.SHA256 {
		t.Fatalf("source hash changed: %q != %q", repeated.SHA256, source.SHA256)
	}
}
