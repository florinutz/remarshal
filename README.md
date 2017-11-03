# remarshal
[![Build Status](https://travis-ci.org/florinutz/remarshal.svg?branch=master)](https://travis-ci.org/florinutz/remarshal) [![Go Report Card](https://goreportcard.com/badge/github.com/florinutz/remarshal)](https://goreportcard.com/report/github.com/florinutz/remarshal)
[![GoDoc](https://godoc.org/github.com/florinutz/remarshal?status.svg)](https://godoc.org/github.com/florinutz/remarshal)
[![Coverage](https://codecov.io/gh/florinutz/remarshal/branch/master/graph/badge.svg)](https://codecov.io/gh/florinutz/remarshal)

The package looks up for values in a string and then attaches them to an existing struct's fields. It exposes one interface `RegexUnmarshaler`

```go
func Remarshal(text string, v interface{}, splitter interface{}) error
```

The splitter can be one of:
* `*regexp.Regexp`
* `func(string) (map[string]string, error)`
* anything that implements `StringValuesMapper`
```go
type StringValuesMapper interface {
	GetStringMap(string) (map[string]string, error)
}
```

## Examples
### regex splitter
```go
v := &struct {
    One   string `regex_group:"first"`
    Two   string // regex_group defaults to Two
    Three string `regex_group:"Two"` // this takes precedence over Two
    Four  string `regex_group:"Three"`
}{}
splitter := regexp.MustCompile(`^(?P<first>.*)\|(?P<Two>.*)\|(?P<Three>.*)\|(?P<Last>.*)$`)

err := remarshal.Remarshal("first|second|third|... and so on", v, splitter)
if err != nil {
    fmt.Println(err)
}

fmt.Printf("%#v", v)
// Output: &struct { One string "regex_group:\"first\""; Two string; Three string "regex_group:\"Two\""; Four string "regex_group:\"Three\"" }{One:"first", Two:"", Three:"second", Four:"third"}

```

### function splitter
```go
v := &struct{ Host, Port string }{}

splitter := func(s string) (map[string]string, error) {
    host, port, _ := net.SplitHostPort(s)
    return map[string]string{
        "Host": host,
        "Port": port,
    }, nil
}

err := remarshal.Remarshal("localhost:12345", v, splitter)
if err != nil {
    fmt.Println(err)
}

fmt.Printf("%#v", v)
// Output: &struct { Host string; Port string }{Host:"localhost", Port:"12345"}
```

Remarshal returns multiple errors packed into one using the [hashicorp/multierror](https://github.com/hashicorp/go-multierror) package. You can unpack them.