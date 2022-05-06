// Package j2n allows arbitrary JSON to be marshaled into structs. Any JSON
// fields that are not marshaled directly into the fields of the struct are put
// into a field with type UnknownFields.
//
// This means that fields that are not explicitly named in the struct will
// survive an Unmarshal/Marshal round trip.
//
// To avoid recursive calls to MarshalJSON/UnmarshalJSON, use the following
// pattern:
//
//  type CatData struct {
//  	Name string        `json_helpers:"name"`
//  	Rest UnknownFields `json_helpers:"-"`
//  }
//
//  type Cat struct {
//  	CatData
//  }
//
//  func (c *Cat) UnmarshalJSON(data []byte) error {
//  	return j2n.UnmarshalJSON(data, &c.CatData)
//  }
//
//  func (c Cat) MarshalJSON() ([]byte, error) {
//  	return j2n.MarshalJSON(c.CatData)
//  }
package j2n

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

type UnknownFields map[string]*json.RawMessage

var unknownFieldsType = reflect.TypeOf((UnknownFields)(nil))

// UnmarshalJSON parses the JSON-encoded data into the struct pointed to by v.
//
// This behaves exactly like json.Unmarshal, but any extra JSON fields that
// are not explicitly named in the struct are unmarshalled in the 'Overflow'
// field.
//
// The struct v must contain a field 'Overflow' of type
//
//	map[string]*json.RawMessage
//
func UnmarshalJSON(data []byte, v interface{}) error {
	overflow, err := resetOverflowMap(v)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &overflow); err != nil {
		return err
	}

	if err := json.Unmarshal(data, v); err != nil {
		return err
	}

	namedFieldsJSON, err := json.Marshal(v)
	if err != nil {
		return err
	}

	namedFieldsMap := make(UnknownFields)
	if err := json.Unmarshal(namedFieldsJSON, &namedFieldsMap); err != nil {
		return err
	}

	for k := range namedFieldsMap {
		delete(overflow, k)
	}

	return nil
}

// MarshalJSON returns the JSON encoding of v, which must be a struct.
//
// This behaves exactly like json.Marshal, but ensures that any extra fields
// mentioned in v.Overflow are output alongside the explicitly named struct
// fields.
//
// It expects v to contain a field named 'Overflow' of type
//
// 	map[string]*json.RawMessage
//
func MarshalJSON(v interface{}) ([]byte, error) {
	result := make(map[string]*json.RawMessage)

	// Do a round trip of the named fields into a map[string]*json_helpers.RawMessage
	namedFieldsJSON, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(namedFieldsJSON, &result)
	if err != nil {
		return nil, err
	}

	overflow, err := getOverflowMap(v)
	if err != nil {
		return nil, err
	}

	for k, v := range overflow {
		if _, ok := result[k]; ok {
			errorText := fmt.Sprintf("named field present in overflow: '%s'", k)
			return nil, errors.New(errorText)
		}
		result[k] = v
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	return resultJSON, nil
}

func resetOverflowMap(v interface{}) (map[string]*json.RawMessage, error) {
	if value, err := getOverflowFieldValue(v); err != nil {
		return nil, err
	} else {
		overflow := make(UnknownFields)
		value.Set(reflect.ValueOf(overflow))
		return overflow, nil
	}
}

func getOverflowMap(v interface{}) (UnknownFields, error) {
	if value, err := getOverflowFieldValue(v); err != nil {
		return nil, err
	} else {
		return value.Interface().(UnknownFields), nil
	}
}

func getOverflowFieldValue(v interface{}) (reflect.Value, error) {
	value := reflect.ValueOf(v)

	// Unwrap the pointer if necessary
	if value.Type().Kind() == reflect.Ptr {
		value = value.Elem()
	}

	// Check that we're dealing with a struct
	if value.Type().Kind() != reflect.Struct {
		errText := fmt.Sprintf("expected struct, got %s", value.Type().Kind())
		return reflect.Value{}, errors.New(errText)
	}

	var field reflect.Value
	var fieldIndex = -1
	for i := 0; i < value.NumField(); i++ {
		f := value.Field(i)

		if f.Type() == unknownFieldsType {
			if fieldIndex == -1 {
				field = f
				fieldIndex = i
			} else {
				return reflect.Value{}, errors.New("multiple unknown fields")
			}
		}
	}

	// Check that we actually found the field
	if fieldIndex == -1 {
		return reflect.Value{}, errors.New("field is not defined")
	}

	// And that it has a tag ensuring that it is omitted from the JSON output
	if val, ok := value.Type().Field(fieldIndex).Tag.Lookup("json"); !ok || val != "-" {
		return reflect.Value{}, errors.New("unknown fields must be ignored by the standard marshaller (use `json:\"-\"`)")
	}

	return field, nil
}
