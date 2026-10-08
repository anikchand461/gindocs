// Package openapi builds an OpenAPI 3.0 document from Gin routes.
package openapi

import (
	"bytes"
	"encoding/json"
)

// Version is the OpenAPI specification version emitted by this package.
const Version = "3.0.3"

// Document is the root of an OpenAPI 3.0 document.
type Document struct {
	OpenAPI    string                `json:"openapi"`
	Info       Info                  `json:"info"`
	Paths      map[string]PathItem   `json:"paths"`
	Components *Components           `json:"components,omitempty"`
	Security   []SecurityRequirement `json:"security,omitempty"`
}

// Info holds API metadata.
type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

// PathItem maps a lowercase HTTP method (get, post, ...) to its operation.
type PathItem map[string]*Operation

// Operation describes a single API operation on a path.
type Operation struct {
	Tags        []string             `json:"tags,omitempty"`
	Summary     string               `json:"summary,omitempty"`
	Description string               `json:"description,omitempty"`
	OperationID string               `json:"operationId,omitempty"`
	Parameters  []Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody         `json:"requestBody,omitempty"`
	Responses   map[string]*Response `json:"responses"`
	Deprecated  bool                 `json:"deprecated,omitempty"`
	// Security is nil to inherit the document's security, or points to an
	// empty slice to mark the operation as public.
	Security *[]SecurityRequirement `json:"security,omitempty"`
}

// Parameter describes a single operation parameter.
type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required"`
	Schema      *Schema `json:"schema"`
}

// RequestBody describes an operation's request body.
type RequestBody struct {
	Required bool                 `json:"required"`
	Content  map[string]MediaType `json:"content"`
}

// MediaType holds the schema for one content type.
type MediaType struct {
	Schema *Schema `json:"schema"`
}

// Response describes a single response from an operation.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// Components holds reusable schemas and security schemes.
type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme describes an authentication method.
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Name         string `json:"name,omitempty"`
	In           string `json:"in,omitempty"`
}

// SecurityRequirement maps security scheme names to required scopes.
type SecurityRequirement map[string][]string

// Schema is the subset of the OpenAPI schema object this package emits.
type Schema struct {
	Ref                  string     `json:"$ref,omitempty"`
	Type                 string     `json:"type,omitempty"`
	Format               string     `json:"format,omitempty"`
	Description          string     `json:"description,omitempty"`
	Enum                 []any      `json:"enum,omitempty"`
	Example              any        `json:"example,omitempty"`
	Minimum              *float64   `json:"minimum,omitempty"`
	Maximum              *float64   `json:"maximum,omitempty"`
	MinLength            *int       `json:"minLength,omitempty"`
	MaxLength            *int       `json:"maxLength,omitempty"`
	MinItems             *int       `json:"minItems,omitempty"`
	MaxItems             *int       `json:"maxItems,omitempty"`
	Items                *Schema    `json:"items,omitempty"`
	Properties           Properties `json:"properties,omitempty"`
	AdditionalProperties *Schema    `json:"additionalProperties,omitempty"`
	Required             []string   `json:"required,omitempty"`
}

// Properties are an object's properties in declaration order.
type Properties []Property

// Property is a named object property.
type Property struct {
	Name   string
	Schema *Schema
}

// MarshalJSON encodes the properties as a JSON object, keeping their order.
func (ps Properties) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, p := range ps {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(p.Name)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(p.Schema)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Get returns the schema of the named property, or nil.
func (ps Properties) Get(name string) *Schema {
	for _, p := range ps {
		if p.Name == name {
			return p.Schema
		}
	}
	return nil
}
