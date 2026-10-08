# gindocs

Automatic Swagger/OpenAPI documentation for [Gin](https://github.com/gin-gonic/gin) with minimal configuration.

Register your routes as usual, add one line, and get interactive API docs at `/docs`. You don't need annotations, code generation, or a `swagger.json` you have to keep up to date.

## Why

Most Swagger tooling for Go (such as `swaggo/swag`) builds the spec from comments on every handler, and you have to regenerate it whenever something changes:

### Before

```go
// GetUser godoc
// @Summary      Get a user
// @Description  Get a user by ID
// @Tags         users
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  User
// @Router       /users/{id} [get]
func GetUser(c *gin.Context) { ... }

// ...repeat for every handler, then run `swag init` and import the generated docs package.
```

Routes and comments drift apart, and new routes don't appear until someone writes their comments.

### With gindocs

```go
r.GET("/users", GetUsers)
r.GET("/users/:id", GetUser)

gindocs.New(r).Serve("/docs")
```

Gin already knows every registered route, so gindocs reads them from `router.Routes()` and builds the OpenAPI document from that list.

## Install

```bash
go get github.com/anikchand461/gindocs
```

## Usage

```go
package main

import (
	"net/http"

	"github.com/anikchand461/gindocs"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "users"})
	})
	r.GET("/users/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	gindocs.New(r).Serve("/docs")

	r.Run(":8080")
}
```

| URL             | Serves                                    |
| --------------- | ----------------------------------------- |
| `/docs`         | Interactive docs UI                       |
| `/openapi.json` | The generated OpenAPI 3.0.3 document      |

The docs UI is a small, dependency-free HTML/CSS/JS page (about 50 KB) embedded in the Go module with `embed`. It works offline and loads nothing from a CDN. It shows:

* **A sidebar** with routes grouped by tag, each with its summary, plus a **Schemas** section. Press `/` to filter.
* **Each endpoint** with its method, path, summary, description, tags, operationId, deprecation, and whether it needs auth.
* **Parameters** with their type, location, required flag and constraints (enum, min/max, length, default, example).
* **Request body and responses** in Example and Schema tabs. Schemas render as an expandable tree, and their types link to their own pages.
* **A schema page** for each model, showing which endpoints use it.
* **An Authorize dialog** for bearer, basic and API-key auth. Credentials are added to requests and kept until the tab closes.
* **A Try it console** with typed inputs (dropdowns for enums and booleans), a JSON body pre-filled from the schema example, and JSON validation. Send with the button or ⌘/Ctrl + Enter. It shows the request URL, status, timing, pretty-printed body (with Download), response headers, and a live cURL command.
* **A server selector** when the spec lists more than one server.
* **Light and dark themes.** The page follows your system setting by default, and a toggle in the header lets you override it. The choice is remembered.
* **Phone layout.** On small screens the route list moves into a slide-out drawer behind a ☰ button, and the header stays pinned to the top.

The spec at `/openapi.json` is standard OpenAPI 3.0.3, so you can also load it into Swagger UI, Postman, Insomnia or a client generator.

## What's generated automatically

With no configuration, every route gets:

| From                | You get                                                                 |
| ------------------- | ----------------------------------------------------------------------- |
| The path            | `/users/:id` → `/users/{id}` with a required string `id` parameter     |
| The handler name    | Summary `GetUserByID` → "Get User By ID", and operationId `getUserByID` |
| The first path segment | Tag `/api/v1/users/:id` → `users` (skips `api` and `v1`-style segments) |

Anonymous handlers (`func(c *gin.Context) {...}`) have no name, so they get no summary, and their operationId comes from the method and path (`getUsersId`).

* Supported methods: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`, and `TRACE`. Methods OpenAPI can't express, such as `CONNECT`, are skipped.
* Undocumented routes have one `default` response, because OpenAPI requires at least one.

## Adding detail where you want it

Request and response types aren't visible from a Gin handler, so you declare them for the routes that matter, using the same structs your handlers already bind:

```go
docs := gindocs.New(r)
docs.Title = "Users API"
docs.Description = "Authorize with the token `demo-token`."
docs.BearerAuth() // or BasicAuth(), APIKey("X-API-Key")

docs.Route("GET /health").Public()
docs.Route("GET /api/v1/users").
	Query(ListUsersQuery{}).   // struct with `form` tags, as for c.ShouldBindQuery
	Response(200, UserList{})
docs.Route("POST /api/v1/users").
	Summary("Create a user").
	Body(CreateUserRequest{}). // as for c.ShouldBindJSON
	Response(201, User{}).
	Response(400, APIError{})
docs.Route("DELETE /api/v1/users/:id").
	Path(UserURI{}).           // struct with `uri` tags, as for c.ShouldBindUri
	Response(204, nil)

docs.Serve("/docs")
```

| `Route` method            | Effect                                                         |
| ------------------------- | -------------------------------------------------------------- |
| `Summary`, `Description`  | Override the summary from the handler name, and add Markdown-lite text |
| `Tags(...)`               | Override the tag from the path                                 |
| `Body(v)`                 | JSON request body schema from `v`'s type                       |
| `Query(v)`, `Path(v)`     | Query parameters / typed path parameters from a struct         |
| `Response(status, v)`     | A response with its JSON schema (`nil` for no body)            |
| `Deprecated()`, `Public()`, `Hide()` | Mark deprecated, exempt from auth, or leave out of the docs |

Patterns accept Gin (`:id`) or OpenAPI (`{id}`) syntax. `Route` panics on a malformed pattern or a non-struct `Query`/`Path`, so mistakes show up at startup.

### How structs become schemas

Named structs become reusable schemas under `components/schemas` and are referenced by name, so recursive types work. Field rules follow `encoding/json` (`json` tags, `-`, embedded structs flattened), and these tags are read:

| Tag                                   | Becomes                                                     |
| ------------------------------------- | ----------------------------------------------------------- |
| `binding:"required"` (or `validate`)  | Required field                                              |
| `binding:"oneof=a b"`                 | `enum`                                                      |
| `binding:"min=1,max=10"` / `gte`, `lte` | `minimum`/`maximum`, `minLength`/`maxLength` or `minItems`/`maxItems` by type |
| `binding:"email"`, `url`, `uuid`      | `format`                                                    |
| `description:"..."`                   | Field description                                           |
| `example:"..."`                       | Example value, parsed to the field's JSON type               |

`time.Time` becomes a `date-time` string, `[]byte` a base64 string, maps become `additionalProperties`, and interfaces accept any value.

### Other options

```go
docs.Version = "2.1.0"           // default "1.0.0"
docs.SpecPath = "/api/spec.json" // default "/openapi.json"
```

`docs.JSON()` returns the spec as JSON, which is handy for writing it to a file in CI.

Routes are read each time the spec is requested, so the docs also include routes registered after `Serve`. The docs' own routes (`/docs`, its assets, and `/openapi.json`) are left out of the spec.

## Known limitations

* **Types must be declared.** Request and response schemas come from `Route(...)`. Gin handlers don't expose them, so they aren't inferred. A `Route` pattern that matches no registered route is silently ignored.
* **JSON bodies only.** Form, multipart and file-upload bodies aren't described yet.
* **Auth applies globally.** `BearerAuth`/`BasicAuth`/`APIKey` cover every route except `Public()` ones. There's no per-route choice between schemes and no OAuth2 flows.
* **Catch-all wildcards.** OpenAPI path parameters can't contain `/`, so `/files/*filepath` becomes a single `{filepath}` parameter. The built-in docs UI sends slashes as typed, so `docs/a.txt` reaches Gin as `/docs/a.txt`. Other OpenAPI tools may encode them as `%2F`, which Gin still decodes as long as `UseRawPath` is off (the default).
* **Absolute URLs.** The docs page loads its assets and spec from absolute paths, so serving it behind a reverse proxy that strips a path prefix won't work yet.

## Development

```bash
go test ./...
go run ./examples/basic   # zero config:          http://localhost:8080/docs
go run ./examples/full    # types + bearer auth:  http://localhost:8081/docs (token: demo-token)
```

### Layout

```text
gindocs.go               Public API: New, Docs, Serve, JSON, BearerAuth, BasicAuth, APIKey
route.go                 Optional per-route documentation (Docs.Route)
internal/openapi/        OpenAPI types, path conversion, naming, struct → schema reflection, generation
internal/ui/             Embedded docs page (index.html, app.css, app.js)
examples/basic/          Zero-config example
examples/full/           Typed bodies, query/path structs, responses and bearer auth
```

### Working on the UI

`internal/ui/app.js` and `app.css` are plain files with no build step. Edit them, restart the example, and reload `/docs`.

## License

MIT. See [LICENSE](LICENSE).
