package remarshal

import (
	"errors"
	"regexp"
)

// StringValuesMapper can parse a string for its values
type StringValuesMapper interface {
	GetStringMap(string) (map[string]string, error)
}

type stringSplitterRegex regexp.Regexp

// This could come handy for instance where you have to use net.SplitHostPort rather than regex
// You'll have to supply the keys manually
type stringSplitterFunc func(string) (map[string]string, error)

// splitter should be a regex.Regexp or stringSplitterFunc-like function
func Split(text string, splitter interface{}) (map[string]string, error) {
	var spl StringValuesMapper = nil
	switch splitter.(type) {
	case func(string) (map[string]string, error):
		aux := splitter.(func(string) (map[string]string, error))
		spl = stringSplitterFunc(aux)
	case regexp.Regexp:
		spl = splitter.(stringSplitterRegex)
	default:
		return nil, errors.New("invalid string splitter")
	}
	return spl.GetStringMap(text)
}

// Computes the regex string map (group => value)
// The error is returned when there was no match
func (rvm stringSplitterRegex) GetStringMap(str string) (result map[string]string, err error) {
	re := regexp.Regexp(rvm)
	match := re.FindStringSubmatch(str)
	if match == nil {
		return nil, errors.New("no match")
	}
	reGroups := re.SubexpNames()[1:]
	result = make(map[string]string)
	for i, value := range match[1:] {
		result[reGroups[i]] = value
	}
	return
}

// Implementing the StringValuesMapper interface for the func
func (f stringSplitterFunc) GetStringMap(text string) (map[string]string, error) {
	return f(text)
}
