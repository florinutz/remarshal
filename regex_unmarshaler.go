/*
Package remarshal uses regex patterns in order to unpack strings into struct properties
and the other way around, in the future.
*/
package remarshal

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"github.com/hashicorp/go-multierror"
)

// Remarshaler describes the main functionality of this package.
// Something that implements it can extract data into the last argument's fields.
type Remarshaler interface {
	Remarshal(string, interface{}, StringValuesMapper) error
}

// StructTag is the custom struct tag name
const StructTag string = "regex_group"

type field struct {
	reflect.StructField
	tagValue         *string
	tagIsSetManually *bool
}

type stringSlice struct {
	Key   string
	Value string
}

type change struct {
	Field       *field
	StringSlice *stringSlice
}

// worker does the heavy lifting behind the exported Remarshal function
type worker struct {
	Text             string
	Splitter         *interface{}
	V                *interface{}
	reflectValue     *reflect.Value
	Values           []*stringSlice
	Fields           []*field
	Changes          []*change
	ExtraFields      []*field
	ExtraRegexGroups []string
}

var workerTemplate *template.Template
var fm = template.FuncMap{
	"join": strings.Join,
}

func init() {
	str := `
* Overview:
	Text: {{.Text}}
	Regex: {{.Re}}
	struct: {{.V}}

* Fields:
	{{range .Fields}}{{.String}}{{$fieldName := .GetTagValue}}{{range $.Values}}{{if eq .Key $fieldName }} => {{.Value}}{{end}}{{end}}
	{{end}}
* Extra regex groups:
	{{join .ExtraRegexGroups ", "}}

* Extra fields
	{{range .ExtraFields}}{{.String}}
	{{end}}
`
	workerTemplate = template.Must(template.New("worker").Funcs(fm).Parse(str))
}

func (field *field) lookupTagIfNeeded() {
	if field.tagValue == nil || field.tagIsSetManually == nil {
		tagValue, setManually := field.Tag.Lookup(StructTag)
		field.tagValue, field.tagIsSetManually = &tagValue, &setManually
	}
}

func (field *field) GetTagValue() string {
	field.lookupTagIfNeeded()
	return *field.tagValue
}

func (field *field) isTagSetManually() bool {
	field.lookupTagIfNeeded()
	return *field.tagIsSetManually
}

// Lookup for interesting fields
func lookupFields(typeOf reflect.Type) (fields []*field, err error) {
	// parsing of fields
	for i := 0; i < typeOf.NumField(); i++ {
		field := makeField(typeOf.Field(i))
		if existingField := field.isAmong(fields); existingField != nil {
			if existingField.isTagSetManually() && field.isTagSetManually() { // conflict for double tag on the struct
				return nil, fmt.Errorf(`you have tag "%s" on both "%s" and "%s"`,
					existingField.GetTagValue(),
					existingField.Name,
					field.Name,
				)
			}
			if !existingField.isTagSetManually() && field.isTagSetManually() {
				existingField.impersonate(field)
			}
		} else {
			fields = append(fields, field)
		}
	}
	return
}

func (field *field) impersonate(targetField *field) {
	field.StructField = targetField.StructField
	field.tagValue = targetField.tagValue
	field.tagIsSetManually = targetField.tagIsSetManually
}

func makeField(f reflect.StructField) *field {
	field := &field{f, nil, nil}
	field.lookupTagIfNeeded()
	if *field.tagValue == "" {
		field.tagValue = &field.Name
	}
	return field
}

// Returns the existing field or nil
func (field *field) isAmong(fields []*field) *field {
	for _, f := range fields {
		if field.GetTagValue() == f.GetTagValue() {
			return f
		}
	}
	return nil
}

func getExtraStringMapKeys(fields []*field, values []*stringSlice) (extra []string) {
	extra = []string{}
	for _, value := range values {
		match := false
		for _, field := range fields {
			if field.GetTagValue() == value.Key {
				match = true
				break
			}
		}
		if !match {
			extra = append(extra, value.Key)
		}
	}
	return
}

func getExtraTags(fields []*field, values []*stringSlice) (extra []*field) {
	for _, field := range fields {
		match := false
		for _, value := range values {
			if field.GetTagValue() == value.Key {
				match = true
			}
		}
		if !match && *field.tagIsSetManually {
			extra = append(extra, field)
		}
	}
	return
}

func getChanges(fields []*field, values []*stringSlice) (changes []*change) {
	for _, value := range values {
		for _, field := range fields {
			if field.GetTagValue() == value.Key {
				changes = append(changes, &change{
					StringSlice: value,
					Field:       field,
				})
			}
		}
	}
	return
}

// applyChanges sets the computed value changeset on the struct
func (worker *worker) applyChanges() (errs []error) {
	value := worker.reflectValue.Elem()
	for _, change := range worker.Changes {
		reflectValue := value.FieldByName(change.Field.Name)
		if !reflectValue.CanSet() {
			errs = append(errs, fmt.Errorf("can't set value '%s' for field '%s'",
				change.StringSlice.Value,
				change.Field.Name,
			))
			continue
		}

		newValue := change.StringSlice.Value
		fieldType, _ := value.Type().FieldByName(change.Field.Name)
		dataType := fieldType.Type.Kind()

		switch dataType {
		case reflect.String:
			reflectValue.SetString(newValue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			converted, err := strconv.ParseInt(newValue, 0, strconv.IntSize)
			if err != nil {
				addConversionError(&errs, change, "int")
			}
			reflectValue.SetInt(converted)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			converted, err := strconv.ParseUint(newValue, 0, strconv.IntSize)
			if err != nil {
				addConversionError(&errs, change, "uint")
			}
			reflectValue.SetUint(converted)
		case reflect.Float32, reflect.Float64:
			converted, err := strconv.ParseFloat(newValue, 0)
			if err != nil {
				addConversionError(&errs, change, "float")
			}
			reflectValue.SetFloat(converted)
		case reflect.Bool:
			converted, err := strconv.ParseBool(newValue)
			if err != nil {
				addConversionError(&errs, change, "bool")
			}
			reflectValue.SetBool(converted)
		default:
			errStr := "%s field's type '%s' unknown, can't assign value '%s' corresponding to regex group '%s'"
			errs = append(errs, fmt.Errorf(errStr,
				change.Field.Name,
				dataType.String(),
				change.StringSlice.Value,
				change.StringSlice.Key,
			))
		}
	}
	return
}

func addConversionError(errs *[]error, change *change, kind string) {
	errStr := "value '%s' of regex group '%s' can't be converted to %s in order to be assigned to field '%s'"
	*errs = append(*errs, fmt.Errorf(errStr,
		change.StringSlice.Value,
		change.StringSlice.Key,
		kind,
		change.Field.Name,
	))
}

func validate(v interface{}) (*reflect.Value, error) {
	// validation
	valueOf := reflect.ValueOf(v)
	if valueOf.Type().Kind() != reflect.Ptr {
		return nil, errors.New("this param is supposed to be a pointer to struct")
	}
	if valueOf.Elem().Type().Kind() != reflect.Struct {
		return nil, errors.New("the value at the end of the pointer is not a struct")
	}
	return &valueOf, nil
}

// newWorker instantiates the worker type, which implements the Remarshaler interface.
// The splitter should come in one of the formats accepted by the Split function.
func newWorker(text string, v interface{}, splitter interface{}) (w *worker, errs []error) {
	var err error
	w = &worker{}

	w.reflectValue, err = validate(v)
	if err != nil {
		errs = append(errs, err)
		return // v is not a pointer to a struct
	}
	fields, err := lookupFields(w.reflectValue.Elem().Type())
	if err != nil {
		// 2 or more tags point to the same re group
		errs = append(errs, err)
	}
	stringMap, err := Split(text, splitter)
	if err != nil {
		// no match
		errs = append(errs, err)
	}
	values := []*stringSlice{}
	for key, val := range stringMap {
		values = append(values, &stringSlice{Key: key, Value: val})
	}

	w.V = &v
	w.Values = values
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
	w.Splitter = &splitter

	return
}

func (worker *worker) String() string {
	var render bytes.Buffer
	err := workerTemplate.ExecuteTemplate(&render, "worker", worker)
	if err != nil {
		return err.Error()
	}
	return render.String()
}

func (field *field) String() string {
	return fmt.Sprintf("%d. %s `%s`", field.Index[0]+1, field.Name, *field.tagValue)
}

func (v *stringSlice) String() string {
	return fmt.Sprintf("%s: %s", v.Key, v.Value)
}

func (change *change) String() string {
	return fmt.Sprintf("%s => %s", change.Field.Name, change.StringSlice.Value)
}

// Remarshal is an example implementation of the Remarshaler interface
func Remarshal(text string, v interface{}, splitter interface{}) error {
	var multiError *multierror.Error
	worker, errs := newWorker(text, v, splitter)
	if len(errs) > 0 {
		// these should be validation errors, so fatal, so let's return
		return multierror.Append(multiError, errs...)
	}
	return multierror.Append(multiError, worker.applyChanges()...).ErrorOrNil()
}
