package circuitgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func (value ParameterValue) MarshalJSON() ([]byte, error) {
	switch {
	case value.String != nil:
		return json.Marshal(*value.String)
	case value.Number != nil:
		return json.Marshal(*value.Number)
	case value.Bool != nil:
		return json.Marshal(*value.Bool)
	case value.List != nil:
		return json.Marshal(value.List)
	default:
		return nil, fmt.Errorf("parameter value is unset")
	}
}

func (value *ParameterValue) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	*value = ParameterValue{}
	switch typed := raw.(type) {
	case string:
		value.String = &typed
	case bool:
		value.Bool = &typed
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return fmt.Errorf("parameter number: %w", err)
		}
		value.Number = &number
	case []any:
		list := make([]string, len(typed))
		for index, item := range typed {
			stringItem, ok := item.(string)
			if !ok {
				return fmt.Errorf("parameter arrays may contain only strings")
			}
			list[index] = stringItem
		}
		value.List = list
	default:
		return fmt.Errorf("parameter value must be string, number, boolean, or string array")
	}
	return nil
}
