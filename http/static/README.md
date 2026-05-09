# http/static

`http/static` serves project-relative static directories and SPA assets on top of `chi`. Import path: `github.com/Ithildur/EiluneKit/http/static`; package name: `static`.

## Quick Start

```go
handler, err := static.SPAHandler("dist", static.Options{
	AppDir: appdir.Options{EnvVar: "APP_HOME"},
})
if err != nil {
	return err
}

r.Handle("/*", handler)
```

For explicit mounting:

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
