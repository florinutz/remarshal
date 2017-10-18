package remarshal

import (
	"fmt"
	"reflect"
)

// StructValuesSetter can apply a set of values over a structs fields by matching values keys with field names
type StructValuesSetter interface {
	ApplyValues(interface{}, map[string]string) []error
}

// Worker does the heavy lifting behind the exported RegexUnmarshal function
type funcWorker struct {
	Text             string
	V                *interface{}
	reflectValue     *reflect.Value
	Values           []*StringSlice
	Fields           []*Field
	Changes          []*Change
	ExtraFields      []*Field
	ExtraRegexGroups []string
}

func NewFuncWorker(
	text string,
	v interface{},
	strMapper StringValuesMapper,
	structValuesSetter StructValuesSetter,
) (w *funcWorker, errs []error) {
	var err error
	w = &funcWorker{}

	values, err := getStringMap(text, strMapper)
	if err != nil {
		errs = append(errs, err)
	}
	w.Values = values

	w.reflectValue, err = validate(v)
	if err != nil {
		errs = append(errs, err)
		return // v is not a pointer to a struct
	}
	w.V = &v

	fields, err := lookupFields(w.reflectValue.Elem().Type())
	if err != nil {
		// 2 or more tags point to the same re group
		errs = append(errs, err)
	}
	w.Fields = fields

	// these are ok, as the user might reuse the regex pattern
	w.ExtraRegexGroups = getExtraStringMapKeys(w.Fields, w.Values)

	// not ok, check your struct tags pls
	w.ExtraFields = getExtraTags(w.Fields, w.Values)
	for _, extraField := range w.ExtraFields {
		errs = append(errs, fmt.Errorf(
			".%s `%s` not found in your pattern",
			extraField.Name,
			*extraField.tagValue,
		))
	}

	w.Changes = getChanges(w.Fields, w.Values)

	// displayed by String()
	w.Text = text

	return
}

func getStringMap(text string, mapper StringValuesMapper) ([]*StringSlice, error) {
	stringMap, err := mapper.GetStringMap(text)
	if err != nil {
		return nil, err
	}
	values := []*StringSlice{}
	for key, val := range stringMap {
		values = append(values, &StringSlice{Key: key, Value: val})
	}
	return values, nil
}
