package openapi

import (
	"regexp"
	"strings"
	"unicode"
)

// Route is a registered HTTP route in Gin syntax, e.g. GET /users/:id.
type Route struct {
	Method string
	Path   string
	// Handler is Gin's name for the last handler, e.g. "main.GetUsers".
	Handler string
}

// methods lists the HTTP methods that OpenAPI 3.0 path items support.
var methods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

// PathParam is a parameter extracted from a Gin path.
type PathParam struct {
	Name     string
	Wildcard bool
}

// ConvertPath converts a Gin path into an OpenAPI path template and returns
// the path parameters it contains, in order of appearance.
//
//	/users/:id           -> /users/{id}           [id]
//	/files/*filepath     -> /files/{filepath}     [filepath*]
//
// Catch-all parameters (*name) are reported with Wildcard set. Paths already
// in OpenAPI form are returned unchanged.
func ConvertPath(path string) (string, []PathParam) {
	segments := strings.Split(path, "/")
	var params []PathParam
	for i, seg := range segments {
		if len(seg) < 2 || (seg[0] != ':' && seg[0] != '*') {
			continue
		}
		name := seg[1:]
		params = append(params, PathParam{Name: name, Wildcard: seg[0] == '*'})
		segments[i] = "{" + name + "}"
	}
	return strings.Join(segments, "/"), params
}

// operationKey returns the OpenAPI operation key for an HTTP method, or false
// if OpenAPI cannot represent it (e.g. CONNECT or custom methods).
func operationKey(method string) (string, bool) {
	key := strings.ToLower(method)
	return key, methods[key]
}

// closurePart matches the compiler's names for closures: func1, or the bare
// counters of nested closures (main.main.func1.1).
var closurePart = regexp.MustCompile(`^(func)?\d+$`)

// HandlerName extracts the function or method name from a Gin handler name:
//
//	main.GetUsers                            -> GetUsers
//	example.com/api/users.(*Handler).List-fm -> List
//	main.main.func1                          -> "" (anonymous)
func HandlerName(handler string) string {
	name := handler[strings.LastIndex(handler, "/")+1:]
	name = strings.TrimSuffix(name, "-fm")
	name = strings.ReplaceAll(name, "[...]", "") // generic instantiations
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return ""
	}
	for _, p := range parts[1:] {
		if closurePart.MatchString(p) {
			return ""
		}
	}
	return parts[len(parts)-1]
}

// Humanize turns an identifier into a title: "GetUserByID" -> "Get User By ID",
// "list_users" -> "List Users".
func Humanize(name string) string {
	var words []string
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' }) {
		words = append(words, splitCamel(part)...)
	}
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// splitCamel splits "getHTTPServer2" into ["get", "HTTP", "Server2"].
func splitCamel(s string) []string {
	r := []rune(s)
	var words []string
	start := 0
	for i := 1; i < len(r); i++ {
		lowerToUpper := unicode.IsLower(r[i-1]) && unicode.IsUpper(r[i])
		acronymEnd := unicode.IsUpper(r[i-1]) && unicode.IsUpper(r[i]) && i+1 < len(r) && unicode.IsLower(r[i+1])
		if lowerToUpper || acronymEnd {
			words = append(words, string(r[start:i]))
			start = i
		}
	}
	return append(words, string(r[start:]))
}

var versionSegment = regexp.MustCompile(`^v\d+$`)

// DefaultTag groups a route by its first meaningful path segment, skipping
// "api", version segments like "v1", and parameters: /api/v1/users/{id} ->
// "users". Routes with no such segment get "default".
func DefaultTag(path string) string {
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || seg == "api" || versionSegment.MatchString(seg) || strings.HasPrefix(seg, "{") {
			continue
		}
		return seg
	}
	return "default"
}

// operationID builds a lowerCamel identifier from a handler name, or from the
// method and path when the handler is anonymous: GET /users/{id} -> getUsersId.
func operationID(method, path, handler string) string {
	if handler != "" {
		r := []rune(handler)
		r[0] = unicode.ToLower(r[0])
		return string(r)
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, w := range strings.FieldsFunc(path, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}
