package openapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type Address struct {
	City string `json:"city"`
}

type Base struct {
	ID        int64     `json:"id" example:"7"`
	CreatedAt time.Time `json:"createdAt"`
}

type User struct {
	Base
	Name     string            `json:"name" binding:"required,min=2,max=50" description:"Full name" example:"Ada"`
	Email    string            `json:"email,omitempty" binding:"required,email"`
	Role     string            `json:"role" binding:"oneof=admin member"`
	Age      *int              `json:"age" binding:"gte=0,lte=150"`
	Tags     []string          `json:"tags"`
	Meta     map[string]any    `json:"meta"`
	Home     Address           `json:"home"`
	Friends  []*User           `json:"friends"`
	Raw      json.RawMessage   `json:"raw"`
	Avatar   []byte            `json:"avatar"`
	Labels   map[string]string `json:"-"`
	internal string
}

func TestSchemaForStruct(t *testing.T) {
	g := newSchemaGen()
	ref := g.For(reflect.TypeFor[User]())
	if ref.Ref != "#/components/schemas/User" {
		t.Fatalf("ref = %q", ref.Ref)
	}
	u := g.defs["User"]
	if u == nil || u.Type != "object" {
		t.Fatalf("User schema = %+v", u)
	}

	var names []string
	for _, p := range u.Properties {
		names = append(names, p.Name)
	}
	want := []string{"id", "createdAt", "name", "email", "role", "age", "tags", "meta", "home", "friends", "raw", "avatar"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("properties = %v, want %v (declaration order, embedded flattened, '-' and unexported skipped)", names, want)
	}
	if !reflect.DeepEqual(u.Required, []string{"name", "email"}) {
		t.Errorf("required = %v", u.Required)
	}

	p := u.Properties.Get
	checks := []struct {
		name string
		ok   bool
	}{
		{"id int64", p("id").Type == "integer" && p("id").Format == "int64" && p("id").Example == int64(7)},
		{"createdAt", p("createdAt").Type == "string" && p("createdAt").Format == "date-time"},
		{"name", p("name").Description == "Full name" && p("name").Example == "Ada" && *p("name").MinLength == 2 && *p("name").MaxLength == 50},
		{"email", p("email").Format == "email"},
		{"role enum", reflect.DeepEqual(p("role").Enum, []any{"admin", "member"})},
		{"age bounds", p("age").Type == "integer" && *p("age").Minimum == 0 && *p("age").Maximum == 150},
		{"tags", p("tags").Type == "array" && p("tags").Items.Type == "string"},
		{"meta", p("meta").Type == "object" && p("meta").AdditionalProperties != nil},
		{"home ref", p("home").Ref == "#/components/schemas/Address"},
		{"friends recursive", p("friends").Items.Ref == "#/components/schemas/User"},
		{"raw any", p("raw").Type == ""},
		{"avatar bytes", p("avatar").Type == "string" && p("avatar").Format == "byte"},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("%s: unexpected schema", c.name)
		}
	}
	if g.defs["Address"] == nil {
		t.Error("Address not registered")
	}
}

func TestSchemaNameCollision(t *testing.T) {
	type User struct{ X int } // same name as the package-level User
	g := newSchemaGen()
	outer := g.For(reflect.TypeFor[User]())
	inner := g.For(reflect.TypeFor[User]())
	if outer.Ref != inner.Ref {
		t.Errorf("same type got two names: %s, %s", outer.Ref, inner.Ref)
	}
	pkgUser := g.For(reflect.TypeFor[*Address]())
	if pkgUser.Ref != "#/components/schemas/Address" {
		t.Errorf("pointer should resolve to the named type, got %s", pkgUser.Ref)
	}

	g = newSchemaGen()
	a := g.For(reflect.TypeFor[userAlias]()) // package-level User
	b := g.For(reflect.TypeFor[User]())      // local User
	if a.Ref == b.Ref {
		t.Errorf("distinct types share a name: %s", a.Ref)
	}
	if len(g.defs) != 3 { // User, Address, and the local User
		t.Errorf("defs = %v", len(g.defs))
	}
}

// userAlias refers to the package-level User from inside a function that
// declares its own User.
type userAlias = User

func TestPropertiesMarshalKeepsOrder(t *testing.T) {
	s := &Schema{Type: "object", Properties: Properties{
		{Name: "z", Schema: &Schema{Type: "string"}},
		{Name: "a", Schema: &Schema{Type: "integer"}},
	}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); !strings.Contains(got, `"properties":{"z":{"type":"string"},"a":{"type":"integer"}}`) {
		t.Errorf("got %s", got)
	}
}

func TestGenerateWithMeta(t *testing.T) {
	type CreateUser struct {
		Name string `json:"name" binding:"required"`
	}
	type ListQuery struct {
		Page  int    `form:"page" binding:"min=1" example:"1"`
		Query string `form:"q" description:"Search text"`
		Sort  string `form:"sort" binding:"required,oneof=asc desc"`
	}
	type UserURI struct {
		ID int `uri:"id" description:"User ID"`
	}
	routes := []Route{
		{Method: "GET", Path: "/users", Handler: "main.ListUsers"},
		{Method: "POST", Path: "/users", Handler: "main.CreateUser"},
		{Method: "GET", Path: "/users/:id", Handler: "main.GetUser"},
		{Method: "GET", Path: "/health", Handler: "main.Health"},
		{Method: "GET", Path: "/internal", Handler: "main.Internal"},
	}
	cfg := Config{
		Security: map[string]*SecurityScheme{"bearerAuth": {Type: "http", Scheme: "bearer"}},
		Meta: map[string]*Meta{
			MetaKey("GET", "/users"):      {Query: ListQuery{}, Responses: map[int]any{200: []User{}}},
			MetaKey("POST", "/users"):     {Summary: "Create", Tags: []string{"admin"}, Body: CreateUser{}, Responses: map[int]any{201: User{}, 204: nil}},
			MetaKey("GET", "/users/{id}"): {Path: &UserURI{}, Deprecated: true},
			MetaKey("GET", "/health"):     {Public: true},
			MetaKey("get", "/internal"):   {Hidden: true},
		},
	}
	doc := Generate(cfg, routes)

	if _, ok := doc.Paths["/internal"]; ok {
		t.Error("hidden route documented")
	}
	if len(doc.Security) != 1 || doc.Components.SecuritySchemes["bearerAuth"] == nil {
		t.Errorf("security = %v", doc.Security)
	}

	list := doc.Paths["/users"]["get"]
	if len(list.Parameters) != 3 {
		t.Fatalf("query params = %+v", list.Parameters)
	}
	page, q, sort := list.Parameters[0], list.Parameters[1], list.Parameters[2]
	if page.Name != "page" || page.In != "query" || page.Required || page.Schema.Type != "integer" || *page.Schema.Minimum != 1 {
		t.Errorf("page = %+v", page)
	}
	if q.Description != "Search text" || q.Schema.Description != "" {
		t.Errorf("q = %+v", q)
	}
	if !sort.Required || len(sort.Schema.Enum) != 2 {
		t.Errorf("sort = %+v", sort)
	}
	if s := list.Responses["200"].Content["application/json"].Schema; s.Type != "array" || s.Items.Ref != "#/components/schemas/User" {
		t.Errorf("200 schema = %+v", s)
	}
	if list.Responses["default"] != nil {
		t.Error("declared responses should replace the default response")
	}

	create := doc.Paths["/users"]["post"]
	if create.Summary != "Create" || create.Tags[0] != "admin" || !create.RequestBody.Required {
		t.Errorf("create = %+v", create)
	}
	if create.RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/CreateUser" {
		t.Errorf("body = %+v", create.RequestBody.Content)
	}
	if r := create.Responses["204"]; r == nil || r.Description != "No Content" || r.Content != nil {
		t.Errorf("204 = %+v", r)
	}
	if create.Responses["201"].Description != "Created" {
		t.Errorf("201 = %+v", create.Responses["201"])
	}

	get := doc.Paths["/users/{id}"]["get"]
	if !get.Deprecated || get.Parameters[0].Schema.Type != "integer" || get.Parameters[0].Description != "User ID" || !get.Parameters[0].Required {
		t.Errorf("get = %+v / %+v", get, get.Parameters[0])
	}

	health := doc.Paths["/health"]["get"]
	if health.Security == nil || len(*health.Security) != 0 {
		t.Errorf("public route security = %v", health.Security)
	}
	if b, _ := json.Marshal(health); !strings.Contains(string(b), `"security":[]`) {
		t.Errorf("public route must marshal security: [], got %s", b)
	}
	if list.Security != nil {
		t.Error("non-public route should inherit document security")
	}

	for _, name := range []string{"User", "CreateUser", "Address"} {
		if doc.Components.Schemas[name] == nil {
			t.Errorf("schema %s missing", name)
		}
	}
}
