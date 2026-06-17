# http/routes

`http/routes` 让路由元数据贴着 handler，并把结果挂载到 `chi`。

## 推荐路径

普通应用代码优先使用 `routes.Blueprint`。它把 handler、元数据、tags 和中间件放在同一处，并让子路由引入更明确。

当路由来自代码生成、需要从其他 router 适配，或需要直接控制 `[]routes.Route` 时，使用更底层的 `routes.Route` 和 `routes.Mount`。`Blueprint` 构建的是同一套路由数据，不是另一套路由系统。

`Blueprint` 使用 handler-required（必填 handler）的注册形状：方法接收 `path`、`summary`、通过 `routes.Func` 或 `routes.Handler` 构造的必填 handler，然后才是路由选项。handler 函数和方法值优先用 `routes.Func(fn)`；只有中间件或适配器已经返回 `http.Handler` 时才用 `routes.Handler(h)`。

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
updater.Get(
	"/remotes/{remoteID}",
	"Get remote",
	routes.Func(remote),
)

api := routes.NewBlueprint()
api.Include("/updater", updater)

err = api.MountAt(r, "/api")
```

handler 可以在 `*http.Request` 后接收动态 path 值。
`routes.Func` 最多支持 10 个动态 path 值。
最终挂载路由里的动态 path 名必须唯一。

```go
func remote(w http.ResponseWriter, r *http.Request, remoteID string) {
	_ = remoteID
}
```

`Blueprint.Routes()` 返回拥有所有权的 `[]routes.Route` 副本，所以调用方仍然可以通过底层函数导出或挂载。

`AuthPublic`、`AuthOptional` 和 `AuthRequired` 会导出为路由元数据。`Mount` 也会在运行时保护 `AuthRequired` 路由，所以认证中间件必须在认证成功后调用 `routes.WithAuthenticated` 标记请求。

## 底层用法

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

同一套路由声明需要生成元数据时，使用 `routes.ExportJSON`、`routes.ExportMarkdown` 或 `routes.ExportOpenAPI`。

```go
spec, err := routes.ExportOpenAPI(api.Routes(), routes.OpenAPIOptions{
	Title:   "Updater API",
	Version: "1.0.0",
})
```
