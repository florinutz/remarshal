package remarshal

import (
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
)

// This can be converted to stringSplitterFunc, which implements StringValuesMapper
func someFuncSplitter(s string) (values map[string]string, err error) {
	aux := strings.Split(s, ",")
	if len(aux) != 2 {
		return nil, errors.New("error")
	}
	values = map[string]string{
		"one": aux[0],
		"two": aux[1],
	}
	return
}

func TestFuncImport(t *testing.T) {
	t.Run("invalid splitter", func(t *testing.T) {
		_, err := Split("text", "notAFunc, nor regexp")
		if err == nil {
			t.Fatal("Error not returned")
		}
	})

	t.Run("func splitter", func(t *testing.T) {
		values, err := Split("one,two", someFuncSplitter)
		if err != nil {
			t.Fatal("error returned by Split function")
		}
		if !reflect.DeepEqual(values, map[string]string{"one": "one", "two": "two"}) {
			t.Fatal("splitter result was corrupted by the Split function")
		}
	})

	t.Run("func splitter error", func(t *testing.T) {
		_, err := Split("one,two,three", someFuncSplitter)
		if err == nil {
			t.Fatal("Error not returned")
		}
	})
}

type GenericSplitter struct{ Host, Port string }

// implements StringValuesMapper
func (spl GenericSplitter) GetStringMap(s string) (map[string]string, error) {
	host, port, _ := net.SplitHostPort(s)
	return map[string]string{
		"Host": host,
		"Port": port,
	}, nil
}

func TestUnknownGenericSplitter(t *testing.T) {
	v := &struct{ Host, Port string }{}
	err := Remarshal("localhost:12345", v, GenericSplitter{})
	if err != nil {
		t.Fatal(err)
	}
}
