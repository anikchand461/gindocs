// Package gindocs serves automatically generated OpenAPI documentation and
// a docs UI for a Gin engine, without per-route annotations.
//
//	r := gin.Default()
//	r.GET("/users/:id", getUser)
//	gindocs.New(r).Serve("/docs")
//
// Routes are read from [gin.Engine.Routes] whenever the spec is requested, so
// routes registered after Serve are included too. Summaries come from handler
// names and tags from paths; use [Docs.Route] to add request and response
// types where you want richer docs.
package gindocs

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anikchand461/gin-swagger-ui/internal/openapi"
	"github.com/anikchand461/gin-swagger-ui/internal/ui"
	"github.com/gin-gonic/gin"
)

// Docs generates and serves API documentation for a Gin engine.
// Set its fields before calling Serve.
type Docs struct {
	// Title is the API title shown in the docs UI. Default: "Gin API".
	Title string
	// Description is shown under the title. Optional.
	Description string
	// Version is the API version (not the OpenAPI version). Default: "1.0.0".
	Version string
	// SpecPath is where the OpenAPI JSON is served. Default: "/openapi.json".
	SpecPath string

	engine *gin.Engine
	// own holds the routes registered by Serve, which are excluded from the spec.
	own      map[string]bool
	meta     map[string]*openapi.Meta
	security map[string]*openapi.SecurityScheme
}

// New returns documentation for the routes of engine. It does not register
// any routes until Serve is called.
func New(engine *gin.Engine) *Docs {
	return &Docs{
		Title:    "Gin API",
		Version:  "1.0.0",
		SpecPath: "/openapi.json",
		engine:   engine,
		own:      map[string]bool{},
		meta:     map[string]*openapi.Meta{},
		security: map[string]*openapi.SecurityScheme{},
	}
}

// Serve registers the docs UI at path (e.g. "/docs") and the OpenAPI JSON at
// d.SpecPath. It panics if the routes conflict with existing ones, as Gin
// does for any duplicate route.
func (d *Docs) Serve(path string) {
	path = "/" + strings.Trim(path, "/")
	assetBase := strings.TrimSuffix(path, "/")

	index, err := ui.Index(d.Title, assetBase, d.SpecPath)
	if err != nil {
		panic("gindocs: render docs page: " + err.Error())
	}

	d.get(d.SpecPath, func(c *gin.Context) {
		spec, err := d.JSON()
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", spec)
	})
	d.get(path, func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
	d.get(assetBase+"/"+ui.CSSFile, func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css; charset=utf-8", ui.CSS)
	})
	d.get(assetBase+"/"+ui.JSFile, func(c *gin.Context) {
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", ui.JS)
	})
}

// JSON returns the OpenAPI 3.0 document for the engine's current routes.
func (d *Docs) JSON() ([]byte, error) {
	var routes []openapi.Route
	for _, r := range d.engine.Routes() {
		if d.own[r.Method+" "+r.Path] {
			continue
		}
		routes = append(routes, openapi.Route{Method: r.Method, Path: r.Path, Handler: r.Handler})
	}
	doc := openapi.Generate(openapi.Config{
		Info:     openapi.Info{Title: d.Title, Description: d.Description, Version: d.Version},
		Security: d.security,
		Meta:     d.meta,
	}, routes)
	return json.MarshalIndent(doc, "", "  ")
}

func (d *Docs) get(path string, h gin.HandlerFunc) {
	d.engine.GET(path, h)
	d.own[http.MethodGet+" "+path] = true
}

// BearerAuth documents that the API expects "Authorization: Bearer <token>".
// It applies to every route except those marked [Route.Public], and adds an
// Authorize button to the docs UI.
func (d *Docs) BearerAuth() {
	d.security["bearerAuth"] = &openapi.SecurityScheme{Type: "http", Scheme: "bearer"}
}

// BasicAuth documents that the API expects HTTP Basic authentication.
// See [Docs.BearerAuth] for how it applies.
func (d *Docs) BasicAuth() {
	d.security["basicAuth"] = &openapi.SecurityScheme{Type: "http", Scheme: "basic"}
}

// APIKey documents that the API expects a key in the named request header,
// e.g. "X-API-Key". See [Docs.BearerAuth] for how it applies.
func (d *Docs) APIKey(header string) {
	d.security["apiKeyAuth"] = &openapi.SecurityScheme{Type: "apiKey", In: "header", Name: header}
}
