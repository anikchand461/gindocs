package openapi

import (
	"reflect"
	"testing"
)

func TestConvertPath(t *testing.T) {
	tests := []struct {
		in         string
		wantPath   string
		wantParams []PathParam
	}{
		{"/users", "/users", nil},
		{"/users/:id", "/users/{id}", []PathParam{{Name: "id"}}},
		{
			"/users/:userID/posts/:postID",
			"/users/{userID}/posts/{postID}",
			[]PathParam{{Name: "userID"}, {Name: "postID"}},
		},
		{"/files/*filepath", "/files/{filepath}", []PathParam{{Name: "filepath", Wildcard: true}}},
		{"/users/{id}", "/users/{id}", nil},
		{"/", "/", nil},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			gotPath, gotParams := ConvertPath(tt.in)
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if !reflect.DeepEqual(gotParams, tt.wantParams) {
				t.Errorf("params = %+v, want %+v", gotParams, tt.wantParams)
			}
		})
	}
}

func TestHandlerName(t *testing.T) {
	tests := map[string]string{
		"main.GetUsers":                                    "GetUsers",
		"example.com/api/users.ListUsers":                  "ListUsers",
		"example.com/api/users.(*Handler).List-fm":         "List",
		"example.com/api/users.Handler.Get-fm":             "Get",
		"example.com/api/users.Page[...]":                  "Page",
		"main.main.func1":                                  "",
		"main.main.func1.1":                                "",
		"main.glob..func2":                                 "",
		"example.com/api/users.(*Handler).List.func3":      "",
		"github.com/gin-gonic/gin.WrapF.func1":             "",
		"github.com/gin-gonic/gin.(*Engine).handleHTTPReq": "handleHTTPReq",
	}
	for in, want := range tests {
		if got := HandlerName(in); got != want {
			t.Errorf("HandlerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanize(t *testing.T) {
	tests := map[string]string{
		"GetUsers":      "Get Users",
		"getUserByID":   "Get User By ID",
		"HTTPServer":    "HTTP Server",
		"list_users":    "List Users",
		"CreateOAuth2":  "Create O Auth2",
		"Health":        "Health",
		"":              "",
		"uploadAPIKeys": "Upload API Keys",
	}
	for in, want := range tests {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultTag(t *testing.T) {
	tests := map[string]string{
		"/users":                  "users",
		"/users/{id}":             "users",
		"/api/v1/orders/{id}":     "orders",
		"/api/v2":                 "default",
		"/":                       "default",
		"/{tenant}/projects":      "projects",
		"/files/{filepath}":       "files",
		"/health":                 "health",
		"/api/users/{id}/avatars": "users",
	}
	for in, want := range tests {
		if got := DefaultTag(in); got != want {
			t.Errorf("DefaultTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerate(t *testing.T) {
	routes := []Route{
		{Method: "GET", Path: "/users", Handler: "main.ListUsers"},
		{Method: "POST", Path: "/users", Handler: "main.main.func1"},
		{Method: "GET", Path: "/users/:id"},
		{Method: "PUT", Path: "/users/:id"},
		{Method: "PATCH", Path: "/users/:id"},
		{Method: "DELETE", Path: "/users/:id"},
		{Method: "HEAD", Path: "/users/:id"},
		{Method: "OPTIONS", Path: "/users/:id"},
		{Method: "GET", Path: "/users/:userID/posts/:postID"},
		{Method: "GET", Path: "/files/*filepath"},
		{Method: "CONNECT", Path: "/tunnel"},
	}
	doc := Generate(Config{Info: Info{Title: "T", Version: "1"}}, routes)

	if doc.OpenAPI != "3.0.3" {
		t.Errorf("openapi = %q", doc.OpenAPI)
	}
	if doc.Components != nil || doc.Security != nil {
		t.Errorf("zero config should add no components or security")
	}

	wantMethods := map[string][]string{
		"/users":                         {"get", "post"},
		"/users/{id}":                    {"get", "put", "patch", "delete", "head", "options"},
		"/users/{userID}/posts/{postID}": {"get"},
		"/files/{filepath}":              {"get"},
	}
	if len(doc.Paths) != len(wantMethods) {
		t.Errorf("got %d paths, want %d (CONNECT should be skipped)", len(doc.Paths), len(wantMethods))
	}
	for path, methods := range wantMethods {
		item, ok := doc.Paths[path]
		if !ok {
			t.Errorf("missing path %q", path)
			continue
		}
		if len(item) != len(methods) {
			t.Errorf("%s: got %d operations, want %d", path, len(item), len(methods))
		}
		for _, m := range methods {
			op := item[m]
			if op == nil {
				t.Errorf("%s: missing %s operation", path, m)
				continue
			}
			if op.Responses["default"] == nil {
				t.Errorf("%s %s: want default response", m, path)
			}
		}
	}

	params := doc.Paths["/users/{userID}/posts/{postID}"]["get"].Parameters
	if len(params) != 2 {
		t.Fatalf("got %d params, want 2", len(params))
	}
	for i, name := range []string{"userID", "postID"} {
		p := params[i]
		if p.Name != name || p.In != "path" || !p.Required || p.Schema == nil || p.Schema.Type != "string" {
			t.Errorf("param %d = %+v, want required string path param %q", i, p, name)
		}
	}

	list := doc.Paths["/users"]["get"]
	if list.Summary != "List Users" || list.OperationID != "listUsers" || !reflect.DeepEqual(list.Tags, []string{"users"}) {
		t.Errorf("GET /users: summary %q, operationId %q, tags %v", list.Summary, list.OperationID, list.Tags)
	}
	if len(list.Parameters) != 0 {
		t.Errorf("/users should have no params, got %+v", list.Parameters)
	}
	create := doc.Paths["/users"]["post"]
	if create.Summary != "" || create.OperationID != "postUsers" {
		t.Errorf("anonymous handler: summary %q, operationId %q", create.Summary, create.OperationID)
	}
}

func TestGenerateOperationIDsAreUnique(t *testing.T) {
	routes := []Route{
		{Method: "GET", Path: "/a", Handler: "main.Handle"},
		{Method: "GET", Path: "/b", Handler: "main.Handle"},
		{Method: "GET", Path: "/c", Handler: "main.Handle"},
	}
	doc := Generate(Config{}, routes)
	seen := map[string]bool{}
	for _, item := range doc.Paths {
		id := item["get"].OperationID
		if seen[id] {
			t.Errorf("duplicate operationId %q", id)
		}
		seen[id] = true
	}
}
