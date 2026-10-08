package openapi

import (
	"encoding/json"
	"mime/multipart"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
	fileType       = reflect.TypeFor[multipart.FileHeader]()
)

// schemaGen converts Go types to schemas. Named struct types are stored once
// in defs and referenced with $ref, which also handles recursive types.
type schemaGen struct {
	defs  map[string]*Schema
	names map[reflect.Type]string
	taken map[string]bool
}

func newSchemaGen() *schemaGen {
	return &schemaGen{
		defs:  map[string]*Schema{},
		names: map[reflect.Type]string{},
		taken: map[string]bool{},
	}
}

// For returns the schema for t.
func (g *schemaGen) For(t reflect.Type) *Schema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case timeType:
		return &Schema{Type: "string", Format: "date-time"}
	case rawMessageType:
		return &Schema{}
	case fileType:
		return &Schema{Type: "string", Format: "binary"}
	}

	switch t.Kind() {
	case reflect.Bool:
		return &Schema{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uintptr:
		return &Schema{Type: "integer"}
	case reflect.Int32, reflect.Uint32:
		return &Schema{Type: "integer", Format: "int32"}
	case reflect.Int64, reflect.Uint64:
		return &Schema{Type: "integer", Format: "int64"}
	case reflect.Float32:
		return &Schema{Type: "number", Format: "float"}
	case reflect.Float64:
		return &Schema{Type: "number", Format: "double"}
	case reflect.String:
		return &Schema{Type: "string"}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string", Format: "byte"}
		}
		return &Schema{Type: "array", Items: g.For(t.Elem())}
	case reflect.Map:
		return &Schema{Type: "object", AdditionalProperties: g.For(t.Elem())}
	case reflect.Struct:
		if t.Name() == "" {
			return g.object(t, "json")
		}
		name, ok := g.names[t]
		if !ok {
			name = g.nameFor(t)
			g.names[t] = name
			g.defs[name] = g.object(t, "json")
		}
		return &Schema{Ref: "#/components/schemas/" + name}
	default:
		// Interfaces, funcs, channels: anything goes.
		return &Schema{}
	}
}

// object builds an inline object schema from t's fields, named by tagKey
// ("json" for JSON bodies, "form" for form bodies).
func (g *schemaGen) object(t reflect.Type, tagKey string) *Schema {
	s := &Schema{Type: "object", Properties: Properties{}}
	for _, f := range structFields(t, tagKey) {
		ps := g.For(f.Type)
		applyTags(ps, f.Tag)
		s.Properties = append(s.Properties, Property{Name: f.Name, Schema: ps})
		if f.Required {
			s.Required = append(s.Required, f.Name)
		}
	}
	return s
}

// hasFile reports whether a form struct has a file field
// (*multipart.FileHeader or a slice of them).
func hasFile(t reflect.Type) bool {
	for _, f := range structFields(t, "form") {
		ft := f.Type
		for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			ft = ft.Elem()
		}
		if ft == fileType {
			return true
		}
	}
	return false
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// nameFor picks a unique component name for a named type: the type name, or
// package_Type when two packages use the same name.
func (g *schemaGen) nameFor(t reflect.Type) string {
	clean := func(s string) string { return strings.Trim(unsafeName.ReplaceAllString(s, "_"), "_") }
	name := clean(t.Name())
	if g.taken[name] {
		pkg := t.PkgPath()
		name = clean(pkg[strings.LastIndex(pkg, "/")+1:] + "_" + t.Name())
	}
	for i, base := 2, name; g.taken[name]; i++ {
		name = base + strconv.Itoa(i)
	}
	g.taken[name] = true
	return name
}

// field is an exported struct field as seen by an encoder or Gin binder.
type field struct {
	Name     string
	Type     reflect.Type
	Tag      reflect.StructTag
	Required bool
}

// structFields lists t's fields named by tagKey ("json", "form" or "uri"),
// flattening embedded structs the way encoding/json does.
func structFields(t reflect.Type, tagKey string) []field {
	var out []field
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get(tagKey)
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")

		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				out = append(out, structFields(ft, tagKey)...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, field{Name: name, Type: f.Type, Tag: f.Tag, Required: isRequired(f.Tag)})
	}
	return out
}

// rules returns the validation rules from Gin's binding tag (or validate).
func rules(tag reflect.StructTag) []string {
	v := tag.Get("binding")
	if v == "" {
		v = tag.Get("validate")
	}
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

func isRequired(tag reflect.StructTag) bool {
	for _, r := range rules(tag) {
		if r == "required" {
			return true
		}
	}
	return false
}

// applyTags copies description, example and validation rules from struct
// tags onto s. $ref schemas are left alone: OpenAPI 3.0 ignores siblings of
// $ref.
func applyTags(s *Schema, tag reflect.StructTag) {
	if s.Ref != "" {
		return
	}
	if d := tag.Get("description"); d != "" {
		s.Description = d
	}
	if ex, ok := tag.Lookup("example"); ok {
		s.Example = parseValue(s.Type, ex)
	}
	for _, r := range rules(tag) {
		key, arg, _ := strings.Cut(r, "=")
		switch key {
		case "email":
			s.Format = "email"
		case "url", "uri", "http_url":
			s.Format = "uri"
		case "uuid", "uuid4":
			s.Format = "uuid"
		case "datetime":
			s.Format = "date-time"
		case "oneof":
			for _, v := range strings.Fields(arg) {
				s.Enum = append(s.Enum, parseValue(s.Type, v))
			}
		case "min", "gte":
			setBound(s, arg, true)
		case "max", "lte":
			setBound(s, arg, false)
		}
	}
}

// setBound applies a min/max rule, which validator interprets by kind:
// length for strings, item count for slices, value for numbers.
func setBound(s *Schema, arg string, isMin bool) {
	n, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return
	}
	i := int(n)
	switch s.Type {
	case "string":
		if isMin {
			s.MinLength = &i
		} else {
			s.MaxLength = &i
		}
	case "array":
		if isMin {
			s.MinItems = &i
		} else {
			s.MaxItems = &i
		}
	case "integer", "number":
		if isMin {
			s.Minimum = &n
		} else {
			s.Maximum = &n
		}
	}
}

// parseValue converts a tag value to the JSON type of the schema, falling
// back to the raw string.
func parseValue(typ, v string) any {
	switch typ {
	case "integer":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	case "number":
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	case "boolean":
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	case "array", "object", "":
		var x any
		if json.Unmarshal([]byte(v), &x) == nil {
			return x
		}
	}
	return v
}
