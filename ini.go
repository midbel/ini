//Package ini provides a Reader to read ini file(s).
//
//The Reader can deal with simple values such as string, int, bool but can also
//deal with more complex type such as slice, map and struct like the toml format
//(although this package doesn't try to be compliant with it).
package ini

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
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

var (
	//DefaultSectionName is the name of the top level section of an ini file.
	//By Default, the value is set to the program
	DefaultSectionName = filepath.Base(os.Args[0])

	//DefaultStrictMode defines how the parser and the reader will behave when a section
	//and/or option will not be found or is duplicated.
	DefaultStrictMode = false
)

//list of identifier that will be directly map to a go predefined value.
var supportedIds = map[string]interface{}{
	"true":  true,
	"yes":   true,
	"false": false,
	"no":    false,
	"null":  nil,
}

type Setter interface {
	Set(string) error
	fmt.Stringer
}

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

//Reader is the type to parse ini file.
type Reader struct {
	//Default top level section Name (by default the value is set to
	//DefaultSectionName).
	Default string

	//Strict governs how errors are handled when setting option 's value to the
	//receiver and/or how not found options/sections are handled (by default the
	//the value is set to DefaultStrictMore).
	Strict bool

	//Set this field to true in order to lookup for options/sections with or
	//without taking into accounts the case of a field and/or a name.
	Insensitive bool

	//Err is the last error catched when parsing an INI by the Reader.
	Err    error
	reader io.Reader
	config *section
	once   sync.Once
}

//NewReader creates a new reader to parse ini file.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		reader:  r,
		Default: DefaultSectionName,
		Strict:  DefaultStrictMode,
	}
}

//Read read from the parsed ini files all the option's values into v (v should
//be, ideally, a struct).
//
//The rules to set a value from an option to a field are as follow:
//
//	* check if the field name matchs an option of the current section. If an
//	  an option is found, set the value to the field (see bellow for more detail)
//
//	* check if the field name matchs a sub-section of the current section. If
//	  a section is found, repeat step one.
//
//	* if not options and/or sections is found, an error is returned if the
//	  the Strict field of the Reader has been set to true else the processing
//	  continue to the next field.
//
//Bellow the rules followed to set the actual option'value to a field:
//
//	* if the current field implements the Setter interface, use the Set method
//	  and returns an error if any.
//
//	* if the current field implements the encoding.TextUnmarshaler interface and
//	  the option's value is a string, use the UnmarshalText method and returns
//	  an error if any.
//
//	* set the value to the current field only if the type of the option value
//	  is the same of the current field.
func (r *Reader) Read(v interface{}) error {
	r.once.Do(r.init)
	if r.Err != nil {
		return r.Err
	}
	if v == nil {
		return nil
	}
	return read(reflect.ValueOf(v).Elem(), r.config, r.Strict)
}

//ReadSection reads the section s from the Reader into v. If the embed config of
//the reader is nil, no error is returned and v is unchanged.
//
//See Read for more detail how option's value are set to value.
func (r *Reader) ReadSection(s string, v interface{}) error {
	r.once.Do(r.init)
	if r.Err != nil {
		return r.Err
	}
	c := r.config.Get(s)
	if c == nil {
		return fmt.Errorf("section %s not found", s)
	}
	return read(reflect.ValueOf(v).Elem(), c, r.Strict)
}

func (r *Reader) init() {
	type named interface {
		Name() string
	}
	section := r.Default
	if section == "" {
		switch r := r.reader.(type) {
		case named:
			base := filepath.Base(r.Name())
			if ext := filepath.Ext(base); ext != "" {
				base = strings.Replace(base, ext, "", 1)
			}
			section = base
		case fmt.Stringer:
			section = r.String()
		}
	}
	if c, err := parse(r.reader, section); err != nil {
		r.Err = err
	} else {
		r.config = c
	}
}

func read(v reflect.Value, s *section, strict bool) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		field := t.Field(i)
		//check if s has field.Name as an option
		o, ok := s.Options[strings.ToLower(field.Name)]
		if ok {
			if err := decode(f, reflect.ValueOf(o)); err != nil && strict {
				return err
			}
			continue
		}
		//check if s has field.Name as a section
		switch name := strings.ToLower(field.Name); f.Kind() {
		case reflect.Struct:
			other, ok := s.Sections[name]
			if !ok {
				continue
			}
			v := reflect.New(f.Type()).Elem()
			if err := read(v, other, strict); err != nil && strict {
				return err
			}
			f.Set(v)
		case reflect.Slice:
			other, ok := s.Sections[name]
			if !ok {
				continue
			}
			for _, s := range other.Sections {
				v := reflect.New(f.Type().Elem()).Elem()
				if err := read(v, s, strict); err != nil && strict {
					return err
				}
				f.Set(reflect.Append(f, v))
			}
		case reflect.Map:
		case reflect.Ptr:
			if err := read(f.Elem(), s, strict); err != nil && strict {
				return err
			}
		default:
			if !strict {
				continue
			} else {
				return fmt.Errorf("missing option/section %s", name)
			}
		}
	}
	return nil
}

func decode(v, other reflect.Value) error {
	if other.Kind() == reflect.Interface {
		other = reflect.ValueOf(other.Interface())
	}
	setter := reflect.TypeOf((*Setter)(nil)).Elem()
	if reflect.PtrTo(v.Type()).Implements(setter) && other.Kind() == reflect.String {
		i := v.Addr().Interface().(Setter)
		if err := i.Set(other.String()); err != nil {
			return err
		}
		f := reflect.ValueOf(i)
		v.Set(reflect.Indirect(f))
		return nil
	}

	text := reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	if reflect.PtrTo(v.Type()).Implements(text) && other.Kind() == reflect.String {
		i := v.Addr().Interface().(encoding.TextUnmarshaler)
		if err := i.UnmarshalText([]byte(other.String())); err != nil {
			return err
		}
		f := reflect.ValueOf(i)
		v.Set(reflect.Indirect(f))
		return nil
	}
	if v.Kind() != other.Kind() {
		return fmt.Errorf("mismatched type. Expected %s, got %s", v.Kind(), other.Kind())
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(other.String())
	case reflect.Bool:
		v.SetBool(other.Bool())
	case reflect.Int:
		v.SetInt(other.Int())
	case reflect.Slice, reflect.Array:
		s := reflect.MakeSlice(v.Type(), 0, other.Len())
		for i := 0; i < other.Len(); i++ {
			value := reflect.New(s.Type().Elem()).Elem()
			if err := decode(value, other.Index(i)); err != nil {
				return err
			}
			s = reflect.Append(s, value)
		}
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		for _, k := range other.MapKeys() {
			value := reflect.New(m.Type().Elem()).Elem()
			if err := decode(value, other.MapIndex(k)); err != nil {
				return err
			}
			m.SetMapIndex(k, value)
		}
		v.Set(m)
	default:
		return fmt.Errorf("unsupported data type %s", v.Kind())
	}
	return nil
}

type section struct {
	Name     string
	Options  map[string]interface{}
	Sections map[string]*section
}

func (s *section) Get(n string) *section {
	if s.Name == n {
		return s
	}
	for _, other := range s.Sections {
		if other.Name == n {
			return other
		}
		if other := other.Get(n); other != nil {
			return other
		}
	}
	return nil
}

func parse(reader io.Reader, name string) (*section, error) {
	lex := new(lexer)
	lex.scan.Init(reader)
	lex.scan.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats

	c := &section{name, make(map[string]interface{}), make(map[string]*section)}
	lex.next()
	if lex.token != leftSquareBracket {
		return c, ErrSyntax{expected: "[", got: lex.text(), pos: lex.scan.Pos()}
	}
	for lex.token != scanner.EOF {
		if err := parseSections(lex, c); err != nil {
			return c, err
		}
	}
	return c, nil
}

func parseSections(lex *lexer, c *section) error {
	lex.next()
	base, parts, err := parseSectionName(lex)
	if err != nil {
		return err
	}
	var s *section
	if base != c.Name {
		if other, ok := c.Sections[base]; !ok {
			s = &section{base, make(map[string]interface{}), make(map[string]*section)}
			c.Sections[base] = s
		} else {
			s = other
		}
		for _, name := range parts {
			other, ok := s.Sections[name]
			if !ok {
				s.Sections[name] = &section{name, make(map[string]interface{}), make(map[string]*section)}
				s = s.Sections[name]
			} else {
				s = other
			}
		}
	} else {
		s = c
	}

	lex.next()
	if lex.token != rightSquareBracket {
		return ErrSyntax{expected: "]", got: lex.text(), pos: lex.scan.Pos()}
	}

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
		value, err := parseOption(lex)
		if err != nil {
			return err
		}
		if _, ok := s.Options[option]; ok {
			return ErrDuplicateOption{option, base}
		}
		s.Options[option] = value
		parseComment(lex)
	}

	return nil
}

func parseSectionName(lex *lexer) (string, []string, error) {
	parts := make([]string, 0)
loop:
	for {
		if lex.token != scanner.Ident {
			return "", nil, ErrSyntax{expected: "identifier", got: lex.text(), pos: lex.scan.Pos()}
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
	if len(parts) > 1 {
		return parts[0], parts[1:], nil
	}
	return parts[0], []string{}, nil
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
		return cleanString(lex.text()), nil
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
			v, err := parseOption(lex)
			if err != nil {
				return nil, err
			}
			values = append(values, v)
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
			v, err := parseOption(lex)
			if err != nil {
				return nil, err
			}
			key, ok := v.(string)
			if !ok {
				return nil, ErrSyntax{expected: "hash keys must be strings", got: fmt.Sprintf("%v", v), pos: lex.scan.Pos()}
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

func cleanString(s string) string {
	s = strings.TrimPrefix(s, "\"")
	s = strings.TrimSuffix(s, "\"")

	r := strings.NewReplacer(`\\`, `\`, `\"`, `"`)
	return r.Replace(s)
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
