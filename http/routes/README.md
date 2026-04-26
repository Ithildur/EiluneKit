# http/routes

`http/routes` keeps route metadata next to handlers and mounts the result on `chi`.

## Recommended Path

Use `routes.Blueprint` in normal application code. It keeps handlers, metadata, tags, and middleware in one place, and makes child route inclusion explicit.

Use lower-level `routes.Route` and `routes.Mount` when routes are generated, adapted from another router, or when you need direct control over the route slice. `Blueprint` builds the same route data; it is not a second routing system.

`Blueprint` uses a handler-required registration shape: methods take `path`, `summary`, a required handler built with `routes.Func` or `routes.Handler`, then route options. Prefer `routes.Func(fn)` for plain `http.HandlerFunc` values and methods; use `routes.Handler(h)` only when middleware or an adapter already returned an `http.Handler`.

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
	routes.Func(refresh),
)

api := routes.NewBlueprint()
api.Include("/updater", updater)

err = api.MountAt(r, "/api")
```

`Blueprint.Routes()` returns owned `[]routes.Route` copies, so callers can still export or mount through the lower-level functions.

`AuthPublic`, `AuthOptional`, and `AuthRequired` are export metadata only. Runtime auth still belongs in middleware.

## Lower Level

```go
routes.Mount(r, "/api", []routes.Route{
	{
		Method:     http.MethodGet,
		Path:       "/status",
		Summary:    "Get status",
		Auth:       routes.AuthRequired,
		Handler:    http.HandlerFunc(status),
		Middleware: []routes.Middleware{bearer},
	},
})
```

Use `routes.ExportJSON` or `routes.ExportMarkdown` when the same route declarations should also produce metadata.
