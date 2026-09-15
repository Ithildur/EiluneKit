# http/static

`http/static` provides project-relative static directories and SPA handlers. Import path: `github.com/Ithildur/EiluneKit/http/static`; package name: `static`.

## Quick Start

```go
spa, err := static.SPAHandler("dist", static.Options{
	AppDir: appdir.Options{EnvVar: "APP_HOME"},
})
if err != nil {
	return err
}

handler, err := routes.NewHandler(api.RoutesAt("/api"), routes.HandlerOptions{
	NotFound: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			response.WriteJSONError(w, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		spa.ServeHTTP(w, r)
	}),
})
if err != nil {
	return err
}
```

Here `api` is the application's Blueprint. Pass `handler` to `http.Server.Handler`. The application selects `/api` and `/api/…` for JSON 404 responses and delegates other paths to the SPA. History fallback applies only to missing paths; other filesystem errors receive the file server's error response.

Use fallback injection when sharing a router with API endpoints. A global `/*` participates in method matching and conflicts with descendant routes; it can consume requests that should reach API 404/405 handlers.

For existing chi integration, explicit mounting remains available at a prefix owned exclusively by the static handler:

```go
if _, err := static.MountSPA(r, "/app", "dist", static.Options{
	Development: true,
}); err != nil {
	return err
}
```

## Resolution Rules

- `relPath` must be a clean project-relative path such as `dist` or `web/dist`
- absolute paths, `.` and `..`, duplicate separators, and dirty paths are rejected with `ErrInvalidProjectPath`
- SPA resolution requires `index.html`
- when `Options.Development` is `false`, discovery uses app-home env/executable sources only
- when `Options.Development` is `true`, discovery also allows the working directory

## Options

- `AppDir`: passed through to `appdir.DiscoverHome`
- `Development`: enables working-directory fallback for local development

## Contracts

- `SPAHandler`, `MountSPA`, `Mount`, `ResolveDir`, and `ResolveSPADir` each take one `Options` struct; use `static.Options{}` for defaults
- when `Options.AppDir.Markers` is empty, the package derives markers from `relPath`
- invalid app-home env overrides fail fast with `appdir.ErrEnvInvalid`; they do not fall back to the working directory
