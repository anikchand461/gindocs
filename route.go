package gindocs

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/anikchand461/gindocs/internal/openapi"
)

// Route adds optional documentation to one route. Every route is documented
// without it; use it only where you want more detail.
//
// pattern is "METHOD /path" in Gin (/users/:id) or OpenAPI (/users/{id})
// syntax. Calling Route again for the same pattern adds to the same entry.
// Patterns that match no registered route are ignored.
//
//	docs.Route("POST /users").
//		Summary("Create a user").
//		Body(CreateUser{}).
//		Response(201, User{}).
//		Response(400, APIError{})
func (d *Docs) Route(pattern string) *Route {
	method, path, ok := strings.Cut(strings.TrimSpace(pattern), " ")
	path = strings.TrimSpace(path)
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		panic(`gindocs: Route pattern must look like "GET /users/:id", got "` + pattern + `"`)
	}
	key := openapi.MetaKey(method, path)
	m := d.meta[key]
	if m == nil {
		m = &openapi.Meta{}
		d.meta[key] = m
	}
	return &Route{m: m}
}

// Route holds the documentation for one route. Its methods return the
// Route so calls can be chained.
type Route struct {
	m *openapi.Meta
}

// Summary sets the one-line summary. Default: the handler's name, e.g.
// GetUserByID becomes "Get User By ID".
func (r *Route) Summary(s string) *Route {
	r.m.Summary = s
	return r
}

// Description sets a longer description. Lines separated by a blank line
// become paragraphs; `code`, **bold** and [links](url) are rendered.
func (r *Route) Description(s string) *Route {
	r.m.Description = s
	return r
}

// Tags sets the groups the route is listed under. Default: the first path
// segment after any "api" or version prefix, e.g. /api/v1/users -> "users".
func (r *Route) Tags(tags ...string) *Route {
	r.m.Tags = tags
	return r
}

// Body documents the JSON request body using the type of v, e.g.
// CreateUser{}. Field names follow `json` tags; `binding:"required"` marks
// required fields.
func (r *Route) Body(v any) *Route {
	if v == nil {
		panic("gindocs: Body needs a value such as CreateUser{}")
	}
	r.m.Body = v
	return r
}

// Query documents query parameters using a struct with `form` tags, the same
// struct you would pass to c.ShouldBindQuery.
func (r *Route) Query(v any) *Route {
	mustStruct("Query", v)
	r.m.Query = v
	return r
}

// Path refines path parameter types and descriptions using a struct with
// `uri` tags, the same struct you would pass to c.ShouldBindUri. Without it,
// path parameters are documented as strings.
func (r *Route) Path(v any) *Route {
	mustStruct("Path", v)
	r.m.Path = v
	return r
}

// Response documents a response status and its JSON body type. Pass nil
// for a response without a body, e.g. Response(204, nil).
func (r *Route) Response(status int, v any) *Route {
	if r.m.Responses == nil {
		r.m.Responses = map[int]any{}
	}
	r.m.Responses[status] = v
	return r
}

// Deprecated marks the route as deprecated.
func (r *Route) Deprecated() *Route {
	r.m.Deprecated = true
	return r
}

// Public marks the route as not requiring the authentication set up with
// BearerAuth, BasicAuth or APIKey.
func (r *Route) Public() *Route {
	r.m.Public = true
	return r
}

// Hide leaves the route out of the docs.
func (r *Route) Hide() *Route {
	r.m.Hidden = true
	return r
}

func mustStruct(method string, v any) {
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("gindocs: %s needs a struct value, got %T", method, v))
	}
}
