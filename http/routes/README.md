# http/routes

`http/routes` keeps route metadata next to handlers and mounts the result on `chi`.

## Recommended Path

Use `routes.Blueprint` in normal application code. It keeps handlers, metadata, tags, and middleware in one place, and makes child route inclusion explicit.

Use lower-level `routes.Route` and `routes.Mount` when routes are generated, adapted from another router, or when you need direct control over the route slice. `Blueprint` builds the same route data; it is not a second routing system.

`Blueprint` methods take `path`, `summary`, a handler function or method value directly, then route options. Go 1.27 generic methods check supported handler signatures at compile time while the stored `Route` remains a plain, non-generic value. Use lower-level `Route.Handler` when an adapter already returned an arbitrary `http.Handler`.

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
updater.Post(
	"/refresh",
	"Refresh updater state",
	refresh,
	routes.OperationID("refreshUpdater"),
	routes.EmptyResponse(http.StatusNoContent, "Updater refreshed"),
	routes.Security(routes.SecurityRequirement{
		{
			Name:   "BearerAuth",
			Type:   routes.SecurityHTTP,
			Scheme: "bearer",
		},
	}),
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

Handlers can accept up to 10 dynamic path values after `*http.Request`.
Dynamic path names must be unique in the final mounted route.

```go
func remote(w http.ResponseWriter, r *http.Request, remoteID string) {
	_ = remoteID
}
```

`Blueprint.Routes()` returns owned `[]routes.Route` copies. Use `RoutesAt` when mounting below a prefix so mounting and contract generation consume the same final paths. Dynamic prefix parameters are added as required string path parameters unless the route already declares their metadata.

`AuthPublic`, `AuthOptional`, and `AuthRequired` are exported as route metadata. `Mount` also guards `AuthRequired` routes at runtime, so auth middleware must mark successful requests with `routes.WithAuthenticated`.

## Application Authentication

Built-in `auth/http` and `auth/rbac/http` are optional. Route registration does not require their token managers, JWT claims, or principals. Applications retain their own login endpoints, session lifecycle, CSRF checks, authorization, and transaction boundaries.

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

Here `sessions`, `withPrincipal`, and the response handlers belong to the application. `MountOptions.Unauthorized` controls the guard response when a required route is reached without the marker; it does not override responses already written by authentication middleware and cannot continue to the endpoint. Nil preserves the default JSON 401. Existing mount functions keep their default behavior.

`AuthPublic` and `AuthOptional` do not remove attached middleware. Mount login endpoints separately or use a public blueprint without required-auth middleware. `AuthRequired` proves authentication only; resource authorization stays with the application. OpenAPI security metadata must describe the actual cookie/header/Bearer scheme independently.

## Lower Level

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

The optional `tools/openapi` package generates deterministic, validated OpenAPI 3.1 JSON from final route metadata:

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

Feed the resulting JSON to any OpenAPI 3.1-compatible TypeScript type and client generator. Kit does not bundle a TypeScript generator.

Handlers remain ordinary `net/http` handlers. Mounting routes does not perform runtime schema validation; generation and document validation happen only when `openapi.Generate` is called.

`routes.ExportJSON` and `routes.ExportMarkdown` generate compact route summaries. OpenAPI output is generated by `openapi.Generate`.
