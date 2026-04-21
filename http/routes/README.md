# http/routes

`http/routes` keeps route metadata next to handlers and mounts the result on `chi`.

## Recommended Path

Use `routes.Blueprint` in normal application code. It keeps handlers, metadata, auth policy, tags, and middleware in one place, and makes child route inclusion explicit.

Use lower-level `routes.Route` and `routes.Mount` when routes are generated, adapted from another router, or when you need direct control over the route slice. `Blueprint` builds the same route data; it is not a second routing system.

`Blueprint` has one route registration shape: methods take `http.Handler`. For plain handler functions, use the standard conversion `http.HandlerFunc(fn)`. The package intentionally does not expose `GetFunc` or similar helpers; two equivalent styles only make route trees harder to read.

## Blueprint

```go
updater := routes.NewBlueprint(
	routes.DefaultTags("updater"),
	routes.DefaultAuth(routes.AuthBearerRequired),
)
updater.Post(
	"/refresh",
	http.HandlerFunc(refresh),
	routes.Summary("Refresh updater state"),
)

api := routes.NewBlueprint()
api.Include("/updater", updater)

err := api.MountAt(r, "/api", routes.WithAuth(authResolver))
```

`Blueprint.Routes()` returns owned `[]routes.Route` copies, so callers can still export or mount through the lower-level functions.

## Lower Level

```go
routes.Mount(r, "/api", []routes.Route{
	{
		Method:  http.MethodGet,
		Path:    "/status",
		Summary: "Get status",
		Auth:    routes.AuthNone,
		Handler: http.HandlerFunc(status),
	},
})
```

Use `routes.ExportJSON` or `routes.ExportMarkdown` when the same route declarations should also produce metadata.
