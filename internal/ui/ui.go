// Package ui embeds the documentation page: a small, dependency-free
// HTML/CSS/JS app that renders an OpenAPI document and can send requests.
package ui

import (
	"bytes"
	_ "embed"
	"html/template"
)

// Asset file names, served next to the index page.
const (
	CSSFile = "app.css"
	JSFile  = "app.js"
)

var (
	//go:embed app.css
	CSS []byte

	//go:embed app.js
	JS []byte

	//go:embed index.html
	indexHTML string

	indexTmpl = template.Must(template.New("index").Parse(indexHTML))
)

// Index renders the docs page. assetBase is the URL prefix the CSS and JS
// assets are served under, and specURL is where the OpenAPI JSON lives.
func Index(title, assetBase, specURL string) ([]byte, error) {
	var buf bytes.Buffer
	err := indexTmpl.Execute(&buf, map[string]string{
		"Title":   title,
		"CSSURL":  assetBase + "/" + CSSFile,
		"JSURL":   assetBase + "/" + JSFile,
		"SpecURL": specURL,
	})
	return buf.Bytes(), err
}
