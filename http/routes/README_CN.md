# http/routes

`http/routes` 让路由元数据贴着 handler，并把结果挂载到 `chi`。

## 推荐路径

普通应用代码优先使用 `routes.Blueprint`。它把 handler、元数据、认证策略、tags 和中间件放在同一处，并让子路由引入更明确。

当路由来自代码生成、需要从其他 router 适配，或需要直接控制 `[]routes.Route` 时，使用更底层的 `routes.Route` 和 `routes.Mount`。`Blueprint` 构建的是同一套路由数据，不是另一套路由系统。

`Blueprint` 使用 handler-required（必填 handler）的注册形状：方法接收 `path`、`summary`、通过 `routes.Func` 或 `routes.Handler` 构造的必填 handler，然后才是路由选项。普通 `http.HandlerFunc` 和方法值优先用 `routes.Func(fn)`；只有中间件或适配器已经返回 `http.Handler` 时才用 `routes.Handler(h)`。

## Blueprint

```go
updater := routes.NewBlueprint(
	routes.DefaultTags("updater"),
	routes.DefaultAuth(routes.AuthBearerRequired),
)
updater.Post(
	"/refresh",
	"Refresh updater state",
	routes.Func(refresh),
)

api := routes.NewBlueprint()
api.Include("/updater", updater)

err := api.MountAt(r, "/api", routes.WithAuth(authResolver))
```

`Blueprint.Routes()` 返回拥有所有权的 `[]routes.Route` 副本，所以调用方仍然可以通过底层函数导出或挂载。

## 底层用法

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

同一套路由声明需要生成元数据时，使用 `routes.ExportJSON` 或 `routes.ExportMarkdown`。
