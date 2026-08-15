package canonicaljsonstream

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

type customScalar int

func (value customScalar) MarshalJSON() ([]byte, error) {
	return []byte(`"custom-` + string(rune('0'+value)) + `"`), nil
}

type streamFixture struct {
	Text       string                     `json:"text"`
	Boolean    bool                       `json:"boolean"`
	Integer    int64                      `json:"integer"`
	Float      float64                    `json:"float"`
	Bytes      []byte                     `json:"bytes"`
	NilSlice   []string                   `json:"nil_slice"`
	EmptySlice []string                   `json:"empty_slice"`
	Map        map[string]any             `json:"map"`
	Pointer    *string                    `json:"pointer"`
	Custom     customScalar               `json:"custom"`
	Raw        json.RawMessage            `json:"raw"`
	Omitted    string                     `json:"omitted,omitempty"`
	Ignored    string                     `json:"-"`
	Array      [2]int                     `json:"array"`
	EmptyArray [0]int                     `json:"empty_array,omitempty"`
	Nested     []map[string]streamFixture `json:"nested"`
}

type EmbeddedPayload struct {
	Value string `json:"value"`
}

type embeddedFixture struct {
	EmbeddedPayload
}

func TestEncodeMatchesJSONMarshal(t *testing.T) {
	pointer := "present"
	fixture := streamFixture{
		Text: "<tag>&\u2028\u2029", Boolean: true, Integer: -42, Float: math.SmallestNonzeroFloat64,
		Bytes: []byte{0, 1, 2, 255}, EmptySlice: []string{}, Pointer: &pointer, Custom: 3,
		Raw: json.RawMessage(` { "valid" : true } `), Array: [2]int{1, 2},
		Map:    map[string]any{"z": nil, "a": []any{1.25, "x"}, "m": map[string]int{"b": 2, "a": 1}},
		Nested: []map[string]streamFixture{{"child": {Text: "nested", Map: map[string]any{}}}},
	}
	want, err := json.Marshal(&fixture)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := Encode(&got, &fixture); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("streamed JSON differs\n got: %s\nwant: %s", got.Bytes(), want)
	}
}

func TestEncodeRejectsCyclesAndUnsupportedValues(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	if err := Encode(&bytes.Buffer{}, cycle); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
	if err := Encode(&bytes.Buffer{}, make(chan int)); err == nil {
		t.Fatal("unsupported channel encoded")
	}
	if err := Encode(&bytes.Buffer{}, embeddedFixture{}); err == nil || !strings.Contains(err.Error(), "anonymous field") {
		t.Fatalf("anonymous-field error = %v", err)
	}
}

func TestEncodeRoundTripsRepresentativeDynamicValues(t *testing.T) {
	values := []any{nil, true, int64(-1), uint64(9), 1.5, "text", []any{nil, "x"}, map[string]any{"b": 2.0, "a": true}}
	for _, value := range values {
		var streamed bytes.Buffer
		if err := Encode(&streamed, value); err != nil {
			t.Fatal(err)
		}
		var got, want any
		if err := json.Unmarshal(streamed.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		marshaled, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(marshaled, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip differs: %#v != %#v", got, want)
		}
	}
}
