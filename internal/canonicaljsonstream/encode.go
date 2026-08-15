// Package canonicaljsonstream writes encoding/json-compatible JSON without
// retaining a complete encoded copy in memory.
package canonicaljsonstream

import (
	"bufio"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	structFieldCache  sync.Map
)

// Encode writes supported values using the same compact representation and
// deterministic field and map-key ordering as encoding/json.Marshal. Unlike
// json.Encoder, it walks aggregates incrementally and does not append a trailing
// newline. Anonymous struct fields are deliberately rejected: the frozen
// evaluator schema does not use them, and approximating encoding/json's field-
// conflict rules would make byte equivalence unsafe.
func Encode(writer io.Writer, value any) error {
	buffered := bufio.NewWriterSize(writer, 64*1024)
	encoder := streamEncoder{writer: buffered, active: map[visit]bool{}}
	if err := encoder.value(reflect.ValueOf(value)); err != nil {
		return err
	}
	return buffered.Flush()
}

type visit struct {
	typ reflect.Type
	ptr uintptr
}

type streamEncoder struct {
	writer io.Writer
	active map[visit]bool
	byte   [1]byte
}

type fieldSpec struct {
	index     int
	name      string
	omitEmpty bool
}

type fieldSet struct {
	fields []fieldSpec
	err    error
}

func (encoder *streamEncoder) writeString(value string) error {
	_, err := io.WriteString(encoder.writer, value)
	return err
}

func (encoder *streamEncoder) writeByte(value byte) error {
	encoder.byte[0] = value
	_, err := encoder.writer.Write(encoder.byte[:])
	return err
}

func (encoder *streamEncoder) marshalScalar(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = encoder.writer.Write(data)
	return err
}

func (encoder *streamEncoder) value(value reflect.Value) error {
	if !value.IsValid() {
		return encoder.writeString("null")
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return encoder.writeString("null")
		}
		return encoder.value(value.Elem())
	}
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Map || value.Kind() == reflect.Slice) && value.IsNil() {
		return encoder.writeString("null")
	}
	if value.CanInterface() && value.Type().Implements(jsonMarshalerType) {
		return encoder.marshalScalar(value.Interface())
	}
	if value.CanInterface() && value.Type().Implements(textMarshalerType) {
		return encoder.marshalScalar(value.Interface())
	}
	if value.CanAddr() && value.Addr().CanInterface() && value.Addr().Type().Implements(jsonMarshalerType) {
		return encoder.marshalScalar(value.Addr().Interface())
	}
	if value.CanAddr() && value.Addr().CanInterface() && value.Addr().Type().Implements(textMarshalerType) {
		return encoder.marshalScalar(value.Addr().Interface())
	}

	switch value.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		return encoder.marshalScalar(value.Interface())
	case reflect.Pointer:
		return encoder.indirect(value)
	case reflect.Struct:
		return encoder.structure(value)
	case reflect.Array:
		return encoder.array(value)
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return encoder.marshalScalar(value.Interface())
		}
		return encoder.aggregate(value, '[', ']')
	case reflect.Map:
		return encoder.mapping(value)
	default:
		return &json.UnsupportedTypeError{Type: value.Type()}
	}
}

func (encoder *streamEncoder) indirect(value reflect.Value) error {
	key := visit{typ: value.Type(), ptr: value.Pointer()}
	if encoder.active[key] {
		return fmt.Errorf("json: unsupported value: encountered a cycle via %s", value.Type())
	}
	encoder.active[key] = true
	defer delete(encoder.active, key)
	return encoder.value(value.Elem())
}

func (encoder *streamEncoder) array(value reflect.Value) error {
	return encoder.aggregate(value, '[', ']')
}

func (encoder *streamEncoder) aggregate(value reflect.Value, opening byte, closing byte) error {
	if value.Kind() == reflect.Slice {
		key := visit{typ: value.Type(), ptr: value.Pointer()}
		if value.Len() > 0 && encoder.active[key] {
			return fmt.Errorf("json: unsupported value: encountered a cycle via %s", value.Type())
		}
		if value.Len() > 0 {
			encoder.active[key] = true
			defer delete(encoder.active, key)
		}
	}
	if err := encoder.writeByte(opening); err != nil {
		return err
	}
	for index := 0; index < value.Len(); index++ {
		if index > 0 {
			if err := encoder.writeString(","); err != nil {
				return err
			}
		}
		if err := encoder.value(value.Index(index)); err != nil {
			return err
		}
	}
	return encoder.writeByte(closing)
}

func (encoder *streamEncoder) structure(value reflect.Value) error {
	if err := encoder.writeString("{"); err != nil {
		return err
	}
	written := 0
	typ := value.Type()
	fields := cachedFields(typ)
	if fields.err != nil {
		return fields.err
	}
	for _, spec := range fields.fields {
		field := value.Field(spec.index)
		if spec.omitEmpty && isEmpty(field) {
			continue
		}
		if written > 0 {
			if err := encoder.writeString(","); err != nil {
				return err
			}
		}
		if err := encoder.marshalScalar(spec.name); err != nil {
			return err
		}
		if err := encoder.writeString(":"); err != nil {
			return err
		}
		if err := encoder.value(field); err != nil {
			return fmt.Errorf("encode %s.%s: %w", typ, typ.Field(spec.index).Name, err)
		}
		written++
	}
	return encoder.writeString("}")
}

func (encoder *streamEncoder) mapping(value reflect.Value) error {
	key := visit{typ: value.Type(), ptr: value.Pointer()}
	if encoder.active[key] {
		return fmt.Errorf("json: unsupported value: encountered a cycle via %s", value.Type())
	}
	encoder.active[key] = true
	defer delete(encoder.active, key)

	type entry struct {
		name  string
		value reflect.Value
	}
	entries := make([]entry, 0, value.Len())
	iterator := value.MapRange()
	for iterator.Next() {
		name, err := mapKey(iterator.Key())
		if err != nil {
			return err
		}
		entries = append(entries, entry{name: name, value: iterator.Value()})
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].name < entries[right].name })
	if err := encoder.writeString("{"); err != nil {
		return err
	}
	for index, current := range entries {
		if index > 0 {
			if err := encoder.writeString(","); err != nil {
				return err
			}
		}
		if err := encoder.marshalScalar(current.name); err != nil {
			return err
		}
		if err := encoder.writeString(":"); err != nil {
			return err
		}
		if err := encoder.value(current.value); err != nil {
			return err
		}
	}
	return encoder.writeString("}")
}

func mapKey(value reflect.Value) (string, error) {
	if value.Kind() == reflect.String {
		return value.String(), nil
	}
	if value.CanInterface() && value.Type().Implements(textMarshalerType) {
		text, err := value.Interface().(encoding.TextMarshaler).MarshalText()
		return string(text), err
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(value.Uint(), 10), nil
	default:
		return "", &json.UnsupportedTypeError{Type: value.Type()}
	}
}

func cachedFields(typ reflect.Type) fieldSet {
	if cached, ok := structFieldCache.Load(typ); ok {
		return cached.(fieldSet)
	}
	result := fieldSet{}
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			result.err = fmt.Errorf("canonical JSON streaming does not support anonymous field %s.%s", typ, field.Name)
			break
		}
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		omitEmpty := false
		for options != "" {
			var option string
			option, options, _ = strings.Cut(options, ",")
			omitEmpty = omitEmpty || option == "omitempty"
		}
		result.fields = append(result.fields, fieldSpec{index: index, name: name, omitEmpty: omitEmpty})
	}
	actual, _ := structFieldCache.LoadOrStore(typ, result)
	return actual.(fieldSet)
}

func isEmpty(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return value.IsZero()
	default:
		return false
	}
}
