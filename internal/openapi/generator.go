package openapi

import (
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Meta is optional, user-declared documentation for one route.
type Meta struct {
	Summary     string
	Description string
	Tags        []string
	Body        any         // request body value, e.g. CreateUser{}
	Query       any         // struct with `form` tags
	Path        any         // struct with `uri` tags
	Responses   map[int]any // status -> body value (nil for no body)
	Deprecated  bool
	Hidden      bool
	Public      bool // no authentication required
}

// MetaKey returns the key a route's Meta is stored under, accepting Gin
// (/users/:id) or OpenAPI (/users/{id}) path syntax.
func MetaKey(method, path string) string {
	p, _ := ConvertPath(path)
	return strings.ToUpper(method) + " " + p
}

// Config holds everything Generate needs besides the routes.
type Config struct {
	Info     Info
	Security map[string]*SecurityScheme
	Meta     map[string]*Meta // keyed by MetaKey
}

// Generate builds an OpenAPI document from the given routes. Routes whose
// method OpenAPI cannot represent are skipped.
func Generate(cfg Config, routes []Route) *Document {
	doc := &Document{
		OpenAPI: Version,
		Info:    cfg.Info,
		Paths:   map[string]PathItem{},
	}
	gen := newSchemaGen()
	ids := map[string]bool{}

	for _, r := range routes {
		key, ok := operationKey(r.Method)
		if !ok {
			continue
		}
		path, params := ConvertPath(r.Path)
		meta := cfg.Meta[strings.ToUpper(r.Method)+" "+path]
		if meta == nil {
			meta = &Meta{}
		}
		if meta.Hidden {
			continue
		}

		op := newOperation(gen, path, params, HandlerName(r.Handler), meta)
		op.OperationID = uniqueID(ids, operationID(r.Method, path, HandlerName(r.Handler)))

		item := doc.Paths[path]
		if item == nil {
			item = PathItem{}
			doc.Paths[path] = item
		}
		item[key] = op
	}

	if len(gen.defs) > 0 || len(cfg.Security) > 0 {
		doc.Components = &Components{SecuritySchemes: cfg.Security}
		if len(gen.defs) > 0 {
			doc.Components.Schemas = gen.defs
		}
	}
	// Any one configured scheme satisfies the API's security.
	for _, name := range sortedKeys(cfg.Security) {
		doc.Security = append(doc.Security, SecurityRequirement{name: {}})
	}
	return doc
}

func newOperation(gen *schemaGen, path string, params []PathParam, handler string, meta *Meta) *Operation {
	op := &Operation{
		Tags:        meta.Tags,
		Summary:     meta.Summary,
		Description: meta.Description,
		Deprecated:  meta.Deprecated,
		Responses:   map[string]*Response{},
	}
	if op.Summary == "" {
		op.Summary = Humanize(handler)
	}
	if len(op.Tags) == 0 {
		op.Tags = []string{DefaultTag(path)}
	}
	if meta.Public {
		op.Security = &[]SecurityRequirement{}
	}

	// Path parameters come from the route; a Path struct can refine them.
	declared := structParams(gen, meta.Path, "uri", "path")
	for _, p := range params {
		param := Parameter{Name: p.Name, In: "path", Required: true, Schema: &Schema{Type: "string"}}
		if p.Wildcard {
			param.Description = "Catch-all path segment; may contain '/'."
		}
		if i := slices.IndexFunc(declared, func(d Parameter) bool { return d.Name == p.Name }); i >= 0 {
			param.Schema = declared[i].Schema
			if declared[i].Description != "" {
				param.Description = declared[i].Description
			}
		}
		op.Parameters = append(op.Parameters, param)
	}
	op.Parameters = append(op.Parameters, structParams(gen, meta.Query, "form", "query")...)

	if meta.Body != nil {
		op.RequestBody = &RequestBody{
			Required: true,
			Content:  jsonContent(gen, meta.Body),
		}
	}

	for status, body := range meta.Responses {
		resp := &Response{Description: http.StatusText(status)}
		if resp.Description == "" {
			resp.Description = "Status " + strconv.Itoa(status)
		}
		if body != nil {
			resp.Content = jsonContent(gen, body)
		}
		op.Responses[strconv.Itoa(status)] = resp
	}
	if len(op.Responses) == 0 {
		// OpenAPI requires at least one response.
		op.Responses["default"] = &Response{Description: "Default response"}
	}
	return op
}

func jsonContent(gen *schemaGen, v any) map[string]MediaType {
	return map[string]MediaType{"application/json": {Schema: gen.For(reflect.TypeOf(v))}}
}

// structParams turns the fields of struct v into parameters located in "in",
// naming them by tagKey the way Gin's binders do.
func structParams(gen *schemaGen, v any, tagKey, in string) []Parameter {
	if v == nil {
		return nil
	}
	var out []Parameter
	for _, f := range structFields(structType(v), tagKey) {
		s := gen.For(f.Type)
		applyTags(s, f.Tag)
		// The description belongs on the parameter, not its schema.
		desc := s.Description
		s.Description = ""
		out = append(out, Parameter{Name: f.Name, In: in, Required: f.Required, Description: desc, Schema: s})
	}
	return out
}

// structType returns the struct type of v, dereferencing pointers.
func structType(v any) reflect.Type {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// uniqueID returns id, or id2, id3, ... if it is already taken.
func uniqueID(taken map[string]bool, id string) string {
	for i, base := 2, id; taken[id]; i++ {
		id = base + strconv.Itoa(i)
	}
	taken[id] = true
	return id
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
