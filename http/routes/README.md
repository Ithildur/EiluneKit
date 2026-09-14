# http/routes

`http/routes` keeps route metadata next to handlers and mounts the result on `chi`.

## Usage

Use `routes.Blueprint` to declare handlers, metadata, tags, and middleware. Use `routes.Route` and `routes.Mount` when working directly with route slices or arbitrary `http.Handler` values.

`Blueprint` methods take `path`, `summary`, a handler function or method value, then route options.

## Paths

Non-empty endpoint paths and route prefixes start with a single `/`: use `Get("/users", ...)` with `MountAt(router, "/api")`. Prefixes cannot end with `/`; use `""` for no prefix. Surrounding whitespace is rejected for both.

Paths are composed as `prefix + path`. An empty endpoint selects the prefix itself; `"/"` selects its trailing-slash endpoint. A combined path cannot be empty: use `"/"` for the HTTP root.

| Prefix | Endpoint path | HTTP path |
|---|---|---|
| `""` | `"/"` | `/` |
| `"/api"` | `""` | `/api` |
| `"/api"` | `"/"` | `/api/` |
| `"/api"` | `"/users"` | `/api/users` |
| `"/api"` | `"/users/"` | `/api/users/` |

`Route.Path` uses the same endpoint syntax. `RoutesAt("/api")` and `WithPrefix("/api", ...)` produce paths such as `/api/users`, ready for mounting with an empty prefix and for document export. Invalid slash or whitespace formatting causes Blueprint declaration/composition methods to panic and mount/export functions to return errors. Empty endpoints may be declared, but composition requires a non-empty combined path; mounting and export reject empty final paths.

Full chi route patterns are checked during mounting; chi's syntax validation panics on malformed patterns such as `/users/{id`. Blueprint declaration/composition and the `ExportJSON`/`ExportMarkdown` summary exports do not validate full route patterns.

Multiple mounts may share a prefix. Kit checks the batch and the target router's `Routes()` snapshot by HTTP method. Duplicate routes and wildcard conflicts panic before the batch is registered, with the conflicting paths in the message.

- `/users/{id}/posts` conflicts with `/users/{name}/profile`: shared parameter positions must use the same name.
- `/files/*` conflicts with `/files/download`, `/files/{name}`, or `/files/`; `/files` may be registered separately.
- `/users/{id}` and `/users/me` may coexist, with the static path taking precedence; HTTP methods are checked independently.

Paths retain chi syntax. Regex and parameter-delimiter branches follow chi's rules; regex normalization only applies chi's anchors and does not infer equivalence between different expressions.

Register application endpoints through Kit to apply these checks. Direct chi registrations bypass them; chi's hidden `Mount` forwarding aliases are outside the check. Pass an existing subrouter to Kit when adding endpoints inside it.

## Blueprint

```go
bearer, err := authhttp.RequireBearer(manager)
if err != nil {
	return err
}

updater := routes.NewBlueprint(
	routes.DefaultTags("updater"),
	routes.DefaultAuth(routes.AuthRequired),
	routes.DefaultMiddleware(bearer),
)
updater.Get(
	"/remotes/{remoteID}",
	"Get remote",
	remote,
	routes.OperationID("getRemote"),
	routes.Parameters(routes.Parameter{
		Name:     "remoteID",
		In:       routes.ParameterPath,
		Required: true,
		Schema:   routes.SchemaOf[string](""),
	}),
	routes.JSONResponse(http.StatusOK, "Remote", routes.SchemaOf[remoteResponse]("Remote")),
	routes.Security(routes.SecurityRequirement{
		{
			Name:   "BearerAuth",
			Type:   routes.SecurityHTTP,
			Scheme: "bearer",
		},
	}),
)

api := routes.NewBlueprint()
api.Include("/updater", updater)

routeList := api.RoutesAt("/api")
err = routes.Mount(r, "", routeList)
```

Handlers can accept up to 15 dynamic path values after `*http.Request`.
Dynamic path names must be unique in the final mounted route.

Values come from `Request.PathValue` in final path order, including dynamic prefixes and `*`. Route middleware can override them with `Request.SetPathValue`, including an empty string.

```go
func remote(w http.ResponseWriter, r *http.Request, remoteID string) {
	_ = remoteID
}
```

`Blueprint.Routes()` and `RoutesAt()` return owned `[]routes.Route` copies. Pass the same final route list to mounting and contract generation. Dynamic prefix parameters default to required string path parameters; explicit metadata takes precedence.

Build and mount Blueprints at startup, then leave routes and middleware configuration unchanged while serving. The same Blueprint can be mounted under different prefixes with independent parameter bindings. Pass required values explicitly to asynchronous tasks; do not access chi's route context or keep using `ResponseWriter` after routing returns.

`AuthPublic`, `AuthOptional`, and `AuthRequired` are exported as route metadata. `Mount` also guards `AuthRequired` routes at runtime, so auth middleware must mark successful requests with `routes.WithAuthenticated`.

## Application Authentication

Authentication middleware can use application-owned sessions and principals or the optional `auth/http` and `auth/rbac/http` packages. The application owns resource authorization and session lifecycle.

Supply a standard middleware and mark the request only after the application's authentication checks have succeeded:

```go
func authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := sessions.Authenticate(r)
		if err != nil {
			writeAuthError(w, r, err)
			return
		}
		ctx := withPrincipal(r.Context(), principal)
		next.ServeHTTP(w, r.WithContext(routes.WithAuthenticated(ctx)))
	})
}

api := routes.NewBlueprint(
	routes.DefaultAuth(routes.AuthRequired),
	routes.DefaultMiddleware(authenticate),
)
api.Get("/account", "Current account", accountHandler)
err := routes.MountWithOptions(router, "", api.RoutesAt("/api"), routes.MountOptions{
	Unauthorized: http.HandlerFunc(writeUnauthorized),
})
```

Here `sessions`, `withPrincipal`, and the response handlers belong to the application. `MountOptions.Unauthorized` handles required routes reached without an authentication marker. It cannot continue to the endpoint or override a response already written by authentication middleware. Nil uses the default JSON 401 response.

`AuthPublic` and `AuthOptional` do not remove attached middleware. Mount login endpoints separately or use a public blueprint without required-auth middleware. `AuthRequired` proves authentication only; resource authorization stays with the application. OpenAPI security metadata must describe the actual cookie/header/Bearer scheme independently.

## Route Slices

```go
routes.Mount(r, "/api", []routes.Route{
	{
		Method:      http.MethodGet,
		Path:        "/status",
		Summary:     "Get status",
		OperationID: "getStatus",
		Auth:        routes.AuthPublic,
		Responses: map[string]routes.Response{
			"200": {
				Description: "Status",
				Content: routes.Content{
					"application/json": routes.SchemaOf[statusResponse]("Status"),
				},
			},
		},
		Handler: http.HandlerFunc(status),
	},
})
```

## OpenAPI 3.1

The optional `tools/openapi` package generates deterministic, validated OpenAPI 3.1 JSON from final route metadata. It does not validate HTTP requests or responses at runtime.

```go
spec, err := openapi.Generate(routeList, openapi.Options{
	Title:   "Updater API",
	Version: "1.0.0",
})
```

Each generated operation requires an explicit, globally unique `OperationID` and at least one response. Path parameters, request bodies, response bodies, and security are declared with route options. Named `SchemaOf` values become stable components.

For application types whose custom JSON encoding differs from reflection, use `JSONSchemaAlias() any` to describe the wire type. For a quantity encoded as a decimal JSON string:

```go
func (ByteQuantity) JSONSchemaAlias() any { return "" }
```

The method describes the schema type and does not change JSON encoding or perform runtime validation. Validate generated schemas against real encoded payloads, especially for custom marshalers.

Response headers are declared on `routes.Response.Headers` and can use named schemas:

```go
routes.Respond("204", routes.Response{
	Description: "Completed",
	Headers: map[string]routes.Header{
		"X-Request-ID": {Required: true, Schema: routes.SchemaOf[string]("")},
	},
})
```

Header names must be valid HTTP tokens and unique ignoring case. Declare `Content-Type` through `Response.Content`, not `Headers`. Header declarations describe the contract; handlers remain responsible for writing the actual headers.

Feed the resulting JSON to an OpenAPI 3.1-compatible TypeScript type and client generator. Use `routes.ExportJSON` or `routes.ExportMarkdown` for compact route summaries.
