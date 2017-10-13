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
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/hashicorp/go-multierror"
)

// RegexUnmarshaler describes the main functionality of this package.
// Something that implements it can extract data into the last argument's fields.
type RegexUnmarshaler interface {
	RegexUnmarshal([]byte, interface{}, *regexp.Regexp) error
}

// StructTag is the custom struct tag name
const StructTag string = "regex_group"

type Field struct {
	reflect.StructField
	tagValue         *string
	tagIsSetManually *bool
}

type StringSlice struct {
	Key   string
	Value string
}

type Change struct {
	Field       *Field
	StringSlice *StringSlice
}

// Worker does the heavy lifting behind the exported RegexUnmarshal function
type Worker struct {
	Text             string
	Re               *regexp.Regexp
	V                *interface{}
	reflectValue     *reflect.Value
	Values           []*StringSlice
	Fields           []*Field
	Changes          []*Change
	ExtraFields      []*Field
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

func (field *Field) lookupTagIfNeeded() {
	if field.tagValue == nil || field.tagIsSetManually == nil {
		tagValue, setManually := field.Tag.Lookup(StructTag)
		field.tagValue, field.tagIsSetManually = &tagValue, &setManually
	}
}

func (field *Field) GetTagValue() string {
	field.lookupTagIfNeeded()
	return *field.tagValue
}

func (field *Field) isTagSetManually() bool {
	field.lookupTagIfNeeded()
	return *field.tagIsSetManually
}

// Lookup for interesting fields
func lookupFields(typeOf reflect.Type) (fields []*Field, err error) {
	// parsing of fields
	for i := 0; i < typeOf.NumField(); i++ {
		field := makeField(typeOf.Field(i))
		if existingField := field.isAmong(fields); existingField != nil {
			if existingField.isTagSetManually() && field.isTagSetManually() { // conflict
				return nil, fmt.Errorf(`regex group "%s" can't point to both "%s" and "%s"`,
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
func (field *Field) impersonate(targetField *Field) {
	field.StructField = targetField.StructField
	field.tagValue = targetField.tagValue
	field.tagIsSetManually = targetField.tagIsSetManually
}

func makeField(f reflect.StructField) *Field {
	field := &Field{f, nil, nil}
	field.lookupTagIfNeeded()
	if *field.tagValue == "" {
		field.tagValue = &field.Name
	}
	return field
}

// Returns the existing field or nil
func (field *Field) isAmong(fields []*Field) *Field {
	for _, f := range fields {
		if field.GetTagValue() == f.GetTagValue() {
			return f
		}
	}
	return nil
}

// Computes the regex string map (group => value)
// The error is returned when there was no match
func stringToValues(data string, re *regexp.Regexp) (values []*StringSlice, err error) {
	match := re.FindStringSubmatch(data)
	if match == nil {
		return nil, errors.New("no regex match")
	}
	reGroups := re.SubexpNames()[1:]
	for i, value := range match[1:] {
		values = append(values, &StringSlice{
			Key:   reGroups[i],
			Value: value,
		})
	}
	return
}

func getExtraStringMapKeys(fields []*Field, values []*StringSlice) (extra []string) {
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

func getExtraTags(fields []*Field, values []*StringSlice) (extra []*Field) {
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

func getChanges(fields []*Field, values []*StringSlice) (changes []*Change) {
	for _, value := range values {
		for _, field := range fields {
			if field.GetTagValue() == value.Key {
				changes = append(changes, &Change{
					StringSlice: value,
					Field:       field,
				})
			}
		}
	}
	return
}

// ApplyChanges sets the computed value changeset on the struct
func (worker *Worker) ApplyChanges() (errs []error) {
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
				errStr := "value '%s' of regex group '%s' can't be converted to int in order to be assigned to field '%s'"
				errs = append(errs, fmt.Errorf(errStr,
					change.StringSlice.Value,
					change.StringSlice.Key,
					change.Field.Name,
				))
				continue
			}
			reflectValue.SetInt(converted)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			converted, err := strconv.ParseUint(newValue, 0, strconv.IntSize)
			if err != nil {
				errStr := "value '%s' of regex group '%s' can't be converted to int in order to be assigned to field '%s'"
				errs = append(errs, fmt.Errorf(errStr,
					change.StringSlice.Value,
					change.StringSlice.Key,
					change.Field.Name,
				))
				continue
			}
			reflectValue.SetUint(converted)
		case reflect.Float32, reflect.Float64:
			converted, err := strconv.ParseFloat(newValue, 0)
			if err != nil {
				errStr := "value '%s' of regex group '%s' can't be converted to float in order to be assigned to field '%s'"
				errs = append(errs, fmt.Errorf(errStr,
					change.StringSlice.Value,
					change.StringSlice.Key,
					change.Field.Name,
				))
				continue
			}
			reflectValue.SetFloat(converted)
		case reflect.Bool:
			converted, err := strconv.ParseBool(newValue)
			if err != nil {
				errStr := "value '%s' of regex group '%s' can't be converted to bool in order to be assigned to field '%s'"
				errs = append(errs, fmt.Errorf(errStr,
					change.StringSlice.Value,
					change.StringSlice.Key,
					change.Field.Name,
				))
				continue
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

// NewWorker instantiates the Worker type, which implements the RegexUnmarshaler interface
func NewWorker(text string, v interface{}, re *regexp.Regexp) (w *Worker, errs []error) {
	var err error
	w = &Worker{}

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
	values, err := stringToValues(text, re)
	if err != nil {
		// no match
		errs = append(errs, err)
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
	w.Re = re

	return
}

func (worker *Worker) String() string {
	var render bytes.Buffer
	err := workerTemplate.ExecuteTemplate(&render, "worker", worker)
	if err != nil {
		return err.Error()
	}
	return render.String()
}

func (field *Field) String() string {
	return fmt.Sprintf("%d. %s `%s`", field.Index[0]+1, field.Name, *field.tagValue)
}

func (v *StringSlice) String() string {
	return fmt.Sprintf("%s: %s", v.Key, v.Value)
}

func (change *Change) String() string {
	return fmt.Sprintf("%s => %s", change.Field.Name, change.StringSlice.Value)
}

// RegexUnmarshal is an example implementation of the RegexUnmarshaler interface
func RegexUnmarshal(text string, v interface{}, re *regexp.Regexp) error {
	var multiError *multierror.Error
	worker, errs := NewWorker(text, v, re)
	if len(errs) > 0 {
		// these should be validation errors, so fatal, so let's return
		return multierror.Append(multiError, errs...)
	}
	return multierror.Append(multiError, worker.ApplyChanges()...).ErrorOrNil()
}
