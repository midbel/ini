package ini

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"text/scanner"
)

const (
	dot                = '.'
	semicolon          = ';'
	eq                 = '='
	coma               = ','
	colon              = ':'
	leftSquareBracket  = '['
	rightSquareBracket = ']'
	leftCurlyBracket   = '{'
	rightCurlyBracket  = '}'
)

var supportedIds = map[string]interface{}{
	"true":  true,
	"yes":   true,
	"false": false,
	"no":    false,
	"null":  nil,
}

type config map[string]section

type section map[string]interface{}


//ErrDuplicateSection is returned when a section is defined more than once in a 
//ini files.
type ErrDuplicateSection string

//Error gives an error message for the duplicated section.
func (d ErrDuplicateSection) Error() string {
	return fmt.Sprintf("duplicate section: %q already defined", d)
}

//ErrDuplicateOption is returned when an option is defined more than once in a 
//specific section of an ini file.
type ErrDuplicateOption struct {
	option  string
	section string
}

//Error gives an error message for the duplicated option.
func (d ErrDuplicateOption) Error() string {
	return fmt.Sprintf("duplicate option: %q already defined in section %q", d.option, d.section)
}

//ErrSyntax is returned when the parser meet an unexpected token in an ini file.
//An unexpected token can be a missing ] to close a section header, an identifier
//instead of an option value and so on.
type ErrSyntax struct {
	expected string
	got      string
	pos      scanner.Position
}

//Error gives an error message for the syntax error, the problematic token, the
//expected one and the position in the ini file.
func (s ErrSyntax) Error() string {
	return fmt.Sprintf("syntax error: expected %q, got %q (line: %s)", s.expected, s.got, s.pos)
}

func Read(r io.Reader, data interface{}) error {
	c, err := Parse(r)
	if err != nil {
		return err
	}
	
	v := reflect.ValueOf(data).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		info := t.Field(i)
		switch tag := info.Tag.Get("ini"); tag {
		case "-":
			continue
		case "":
		default:
			var section, option string
			if ix := strings.Index(tag, ">"); ix < 0 {
				section, option = tag, strings.ToLower(info.Name)
			} else {
				section, option = tag[:ix], tag[ix+1:]
			}
			s, ok := c[section]
			if !ok {
				return fmt.Errorf("section %s not found", section)
			}
			other, ok := s[option]
			if !ok {
				return fmt.Errorf("option %s not found in %s", option, section)
			}
			if err := update(f, other); err != nil {
				return err
			}
		}
	}

	return nil
}

func update(f, other reflect.Value) error {
	if other.Kind() == reflect.Interface {
		other = reflect.ValueOf(other.Interface())
	}
	if f.Kind() != other.Kind() {
		return fmt.Errorf("wrong option type: expected %s, got %s", f.Kind(), other.Kind())
	}

	switch k := f.Kind(); k {
	case reflect.Slice, reflect.Array:
		s := reflect.MakeSlice(f.Type(), 0, other.Len())
		for i := 0; i < other.Len(); i++ {
			value := reflect.New(s.Type().Elem()).Elem()
			if err := update(value, other.Index(i)); err != nil {
				return err
			}
			s = reflect.Append(s, value)
		}
		f.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(f.Type())
		for _, k := range other.MapKeys() {
			value := reflect.New(m.Type().Elem()).Elem()
			if err := update(value, other.MapIndex(k)); err != nil {
				return err
			}
			m.SetMapIndex(k, value)
		}
		f.Set(m)
	case reflect.String, reflect.Int, reflect.Bool:
		f.Set(other)
	default:
		return fmt.Errorf("unsupported type %q", k)
	}
	return nil
}

func Parse(reader io.Reader) (config, error) {
	lex := new(lexer)
	lex.scan.Init(reader)
	lex.scan.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats

	c := make(config)
	lex.next()
	if lex.token != leftSquareBracket {
		return c, ErrSyntax{expected: "[", got: lex.text(), pos: lex.scan.Pos()}
	}
	for lex.token != scanner.EOF {
		if err := parse(lex, c); err != nil {
			return c, err
		}
	}

	return c, nil
}

func parseSectionName(lex *lexer) (string, error) {
	parts := make([]string, 0)
loop:
	for {
		if lex.token != scanner.Ident {
			return "", ErrSyntax{expected: "identifier", got: lex.text(), pos: lex.scan.Pos()}
		}
		parts = append(parts, lex.text())
		switch lex.peek() {
		case dot:
			lex.next()
		case rightSquareBracket:
			break loop
		}
		lex.next()
	}
	return strings.Join(parts, "."), nil
}

func parse(lex *lexer, c config) error {
	lex.next()
	name, err := parseSectionName(lex)
	if err != nil {
		return err
	}
	if _, ok := c[name]; ok {
		return ErrDuplicateSection(name)
	}

	lex.next()
	if lex.token != rightSquareBracket {
		return ErrSyntax{expected: "]", got: lex.text(), pos: lex.scan.Pos()}
	}

	s := make(section)
	for {
		lex.next()
		parseComment(lex)
		if lex.token != scanner.Ident {
			if lex.token == leftSquareBracket || lex.token == scanner.EOF {
				break
			}
			return ErrSyntax{expected: "option's key", got: lex.text(), pos: lex.scan.Pos()}
		}
		option := lex.text()
		lex.next()
		if lex.token != eq {
			return ErrSyntax{expected: string(eq), got: lex.text(), pos: lex.scan.Pos()}
		}
		lex.next()
		if value, err := parseOption(lex); err != nil {
			return err
		} else {
			if _, ok := s[option]; ok {
				return ErrDuplicateOption{option, name}
			}
			s[option] = value
			parseComment(lex)
		}
	}
	if len(s) > 0 {
		c[name] = s
	}

	return nil
}

func parseComment(lex *lexer) {
	if lex.token != semicolon {
		return
	}

	lex.next()
	for !lex.IsZero() {
		lex.next()
	}
}

func parseOption(lex *lexer) (interface{}, error) {
	switch lex.token {
	case scanner.Ident:
		id := lex.text()
		if v, ok := supportedIds[id]; ok {
			return v, nil
		}
		return nil, fmt.Errorf("%q unknown identifier", id)
	case scanner.String:
		return strings.Trim(lex.text(), "\""), nil
	case scanner.Int:
		return strconv.Atoi(lex.text())
	case scanner.Float:
		return strconv.ParseFloat(lex.text(), 64)
	case leftSquareBracket:
		values := make([]interface{}, 0)
		for {
			lex.next()
			if lex.token == rightSquareBracket {
				break
			}
			if v, err := parseOption(lex); err != nil {
				return nil, err
			} else {
				values = append(values, v)
			}
			lex.next()
			if lex.token != coma {
				return nil, ErrSyntax{expected: string(coma), got: lex.text(), pos: lex.scan.Pos()}
			}
		}
		return values, nil
	case leftCurlyBracket:
		values := make(map[string]interface{})
		for {
			lex.next()
			if lex.token == rightCurlyBracket {
				break
			}

			var key string
			if v, err := parseOption(lex); err != nil {
				return nil, err
			} else {
				v, ok := v.(string)
				if !ok {
					return nil, ErrSyntax{expected: "hash keys must be strings", got: v, pos: lex.scan.Pos()}
				}
				key = v
			}
			lex.next()
			if lex.token != colon {
				return nil, ErrSyntax{expected: string(colon), got: lex.text(), pos: lex.scan.Pos()}
			}
			lex.next()
			if v, err := parseOption(lex); err != nil {
				return nil, err
			} else {
				values[key] = v
			}
			lex.next()
			if lex.token != coma {
				return nil, ErrSyntax{expected: string(coma), got: lex.text(), pos: lex.scan.Pos()}
			}
		}
		return values, nil
	}
	return nil, ErrSyntax{expected: "option's value", got: lex.text(), pos: lex.scan.Pos()}
}

type lexer struct {
	scan  scanner.Scanner
	token rune
}

func (l *lexer) peek() rune {
	return l.scan.Peek()
}

func (l *lexer) next() {
	l.token = l.scan.Scan()
}

func (l *lexer) text() string {
	return l.scan.TokenText()
}

func (l *lexer) IsZero() bool {
	p := l.scan.Position
	return p.Column == 1
}
