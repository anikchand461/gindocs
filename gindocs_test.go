package gindocs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.GET("/users", h)
	r.GET("/users/:id", h)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestServe(t *testing.T) {
	r := newEngine()
	New(r).Serve("/docs")
	// Registered after Serve: must still appear in the spec.
	r.POST("/late", func(c *gin.Context) {})

	w := get(r, "/openapi.json")
	if w.Code != http.StatusOK {
		t.Fatalf("/openapi.json status = %d", w.Code)
	}
	var spec struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.OpenAPI != "3.0.3" {
		t.Errorf("openapi = %q", spec.OpenAPI)
	}
	for _, p := range []string{"/users", "/users/{id}", "/late"} {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("spec missing %s", p)
		}
	}
	for p := range spec.Paths {
		if p == "/openapi.json" || strings.HasPrefix(p, "/docs") {
			t.Errorf("spec should not document its own route %s", p)
		}
	}

	w = get(r, "/docs")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"/openapi.json"`) {
		t.Errorf("/docs: status %d, body does not reference spec", w.Code)
	}
	for _, asset := range []string{"/docs/app.css", "/docs/app.js"} {
		if w := get(r, asset); w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("%s: status %d, %d bytes", asset, w.Code, w.Body.Len())
		}
	}
}

type testUser struct {
	ID   int    `json:"id"`
	Name string `json:"name" binding:"required"`
}

func ListTestUsers(c *gin.Context) {}

func TestRouteMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users", ListTestUsers)
	r.POST("/users", func(c *gin.Context) {})
	r.GET("/health", func(c *gin.Context) {})

	d := New(r)
	d.Description = "Test API"
	d.BearerAuth()
	d.Route("POST /users").Summary("Create a user").Body(testUser{}).Response(201, testUser{})
	d.Route("GET /health").Public()
	d.Serve("/docs")

	var spec struct {
		Info struct {
			Description string `json:"description"`
		} `json:"info"`
		Paths map[string]map[string]struct {
			Summary     string           `json:"summary"`
			RequestBody *json.RawMessage `json:"requestBody"`
			Security    *[]any           `json:"security"`
		} `json:"paths"`
		Components struct {
			Schemas         map[string]any `json:"schemas"`
			SecuritySchemes map[string]any `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(get(r, "/openapi.json").Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Info.Description != "Test API" {
		t.Errorf("description = %q", spec.Info.Description)
	}
	if got := spec.Paths["/users"]["get"].Summary; got != "List Test Users" {
		t.Errorf("summary from handler name = %q", got)
	}
	if op := spec.Paths["/users"]["post"]; op.Summary != "Create a user" || op.RequestBody == nil {
		t.Errorf("POST /users = %+v", op)
	}
	if s := spec.Paths["/health"]["get"].Security; s == nil || len(*s) != 0 {
		t.Errorf("health should be public")
	}
	if spec.Components.Schemas["testUser"] == nil || spec.Components.SecuritySchemes["bearerAuth"] == nil {
		t.Errorf("components = %+v", spec.Components)
	}
}

func TestRoutePanicsOnBadInput(t *testing.T) {
	d := New(gin.New())
	for name, f := range map[string]func(){
		"bad pattern":     func() { d.Route("/users") },
		"query":           func() { d.Route("GET /users").Query("page") },
		"nil body":        func() { d.Route("POST /users").Body(nil) },
		"nil path":        func() { d.Route("GET /users/:id").Path(nil) },
		"form not struct": func() { d.Route("POST /upload").Form("file") },
		"body then form":  func() { d.Route("POST /a").Body(testUser{}).Form(testUser{}) },
		"form then body":  func() { d.Route("POST /b").Form(testUser{}).Body(testUser{}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: expected panic", name)
				}
			}()
			f()
		}()
	}
}
