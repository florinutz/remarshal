package remarshal

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDataTypes(t *testing.T) {
	v := &struct {
		String  string
		Bool    bool
		Int16   int16
		Uint64  uint64
		Float32 float32
		Float64 float64
	}{}
	err := RegexUnmarshal(
		"string|true|42|42|42.7|42.9",
		v,
		regexp.MustCompile(`^(?P<String>.*)\|(?P<Bool>.*)\|(?P<Int16>.*)\|(?P<Uint64>.*)\|(?P<Float32>.*)\|(?P<Float64>.*)$`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestBasicFunctionality(t *testing.T) {
	v := &struct {
		Smth     string `regex_group:"Something"`
		SmthElse string
	}{}
	err := RegexUnmarshal("a|b", v, regexp.MustCompile(`^(?P<SomethingElse>.*)\|(?P<Something>.*)$`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Smth != "b" || v.SmthElse != "" {
		t.Fatal("Basic functionality failure")
	}
}

func TestInvalidStructTag(t *testing.T) {
	err := RegexUnmarshal(
		"a|b",
		&struct {
			Smth string `regex_group:"boo"`
		}{},
		regexp.MustCompile(`^(?P<SomethingElse>.*)\|(?P<Something>.*)$`))
	if err == nil || !strings.Contains(err.Error(), "boo") {
		t.Fatal("Invalid regex group 'boo' not detected")
	}
}

func TestCrossingTag(t *testing.T) {
	v := &struct {
		Something string
		Smth      string `regex_group:"Something"`
	}{}
	err := RegexUnmarshal("a|b", v, regexp.MustCompile(`^(?P<SomethingElse>.*)\|(?P<Something>.*)$`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Smth != "b" && v.Something != "" {
		t.Fatal("Problem regarding tag's priority over the field name")
	}
}

func TestTagsConflict(t *testing.T) {
	v := &struct {
		Something string `regex_group:"Something"`
		Smth      string `regex_group:"Something"`
	}{}
	err := RegexUnmarshal("a|b", v, regexp.MustCompile(`^(?P<SomethingElse>.*)\|(?P<Something>.*)$`))
	if err == nil {
		t.Fatal("Tag conflict missed detection")
	}
}

func TestUnsettable(t *testing.T) {
	v := &struct{ something string }{}
	splitter := func(string) (map[string]string, error) {
		return map[string]string{"something": "not found"}, nil
	}
	err := RegexUnmarshal("a", v, splitter)
	if err == nil {
		t.Fatal("There should be an error")
	}
	str := err.Error()
	if !strings.Contains(str, "can't set value") {
		t.Fatal("shouldn't be able to set the value here")
	}
}

func ExampleRegexUnmarshal() {
	v := &struct {
		One   string `regex_group:"first"`
		Two   string // regex_group defaults to Two
		Three string `regex_group:"Two"` // this takes precedence over Two
		Four  string `regex_group:"Three"`
	}{}
	re := regexp.MustCompile(`^(?P<first>.*)\|(?P<Two>.*)\|(?P<Three>.*)\|(?P<Last>.*)$`)

	err := RegexUnmarshal("first|second|third|... and so on", v, re)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("%#v", v)
	// Output: &struct { One string "regex_group:\"first\""; Two string; Three string "regex_group:\"Two\""; Four string "regex_group:\"Three\"" }{One:"first", Two:"", Three:"second", Four:"third"}
}

func BenchmarkRegexUnmarshal(b *testing.B) {
	v := &struct {
		One   string `regex_group:"first"`
		Two   string // regex_group defaults to Two
		Three string `regex_group:"Two"` // this takes precedence over Two
		Four  string `regex_group:"Three"`
	}{}
	re := regexp.MustCompile(`^(?P<first>.*)\|(?P<Two>.*)\|(?P<Three>.*)\|(?P<Last>.*)$`)

	for i := 0; i < b.N; i++ {
		RegexUnmarshal("first|second|third|... and so on", v, re)
	}
}
