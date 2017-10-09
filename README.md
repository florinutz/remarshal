# remarshal
[![Build Status](https://travis-ci.org/florinutz/remarshal.svg?branch=master)](https://travis-ci.org/florinutz/remarshal) [![Go Report Card](https://goreportcard.com/badge/github.com/florinutz/remarshal)](https://goreportcard.com/report/github.com/florinutz/remarshal)
[![GoDoc](https://godoc.org/github.com/florinutz/remarshal?status.svg)](https://godoc.org/github.com/florinutz/remarshal)

The package looks up for values in a string and then attaches them to an existing struct's fields. It exposes one interface `RegexUnmarshaler`

```go
type RegexUnmarshaler interface {
	RegexUnmarshal([]byte, *regexp.Regexp, interface{}) error
}
```

and a function that implements it

```go
func RegexUnmarshal(text string, re *regexp.Regexp, v interface{}) error
```

## Example
```bash
go get github.com/florinutz/remarshal
```
then
```go
import "github.com/florinutz/remarshal"
```
and
```go
v := &struct {
    One   string `regex_group:"first"`
    Two   string // regex_group defaults to Two
    Three string `regex_group:"Two"` // this takes precedence over Two
    Four  string `regex_group:"Three"`
}{}
re := regexp.MustCompile(`^(?P<first>.*)\|(?P<Two>.*)\|(?P<Three>.*)\|(?P<Last>.*)$`)

err := remarshal.RegexUnmarshal("first|second|third|... and so on", re, v)
if err != nil {
    fmt.Println(err)
}

fmt.Printf("%#v", v)
// Output: &struct { One string "regex_group:\"first\""; Two string; Three string "regex_group:\"Two\""; Four string "regex_group:\"Three\"" }{One:"first", Two:"", Three:"second", Four:"third"}

```

Please see the [tests](https://github.com/florinutz/remarshal/blob/master/remarshal_test.go) for more details.