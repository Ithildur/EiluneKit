# http/routes

`http/routes` 让路由元数据贴着 handler，并把结果挂载到 `chi`。

## 推荐路径

普通应用代码优先使用 `routes.Blueprint`。它把 handler、元数据、tags 和中间件放在同一处，并让子路由引入更明确。

当路由来自代码生成、需要从其他 router 适配，或需要直接控制 `[]routes.Route` 时，使用更底层的 `routes.Route` 和 `routes.Mount`。`Blueprint` 构建的是同一套路由数据，不是另一套路由系统。

`Blueprint` 方法直接接收 `path`、`summary`、handler 函数或方法值，然后才是路由选项。Go 1.27 泛型方法会在编译期检查支持的 handler 签名，存储的 `Route` 仍是普通的非泛型值。适配器已经返回任意 `http.Handler` 时，使用更底层的 `Route.Handler`。

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

handler 可以在 `*http.Request` 后接收最多 10 个动态 path 值。
最终挂载路由里的动态 path 名必须唯一。

```go
func remote(w http.ResponseWriter, r *http.Request, remoteID string) {
	_ = remoteID
}
```

`Blueprint.Routes()` 返回拥有所有权的 `[]routes.Route` 副本。路由挂在前缀下时使用 `RoutesAt`，让挂载和契约生成消费同一组最终 path。动态前缀参数会成为必填的 string path 参数；路由已显式声明其元数据时以显式声明为准。

`AuthPublic`、`AuthOptional` 和 `AuthRequired` 会导出为路由元数据。`Mount` 也会在运行时保护 `AuthRequired` 路由，所以认证中间件必须在认证成功后调用 `routes.WithAuthenticated` 标记请求。

## 应用认证接入

内置 `auth/http` 和 `auth/rbac/http` 是可选模块。路由注册不要求使用它们的 token manager、JWT claims 或 principal。应用继续拥有登录端点、会话生命周期、CSRF 检查、授权和事务边界。

传入标准中间件，只在应用认证检查成功后标记请求：

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

这里的 `sessions`、`withPrincipal` 和响应 handler 由应用提供。`MountOptions.Unauthorized` 控制必需认证路由缺少标记时的保护响应；它不会覆盖认证中间件已经写出的响应，也不能继续执行端点。Nil 保留默认 JSON 401。现有挂载函数保持默认行为。

`AuthPublic` 和 `AuthOptional` 不会移除已经附加的中间件。登录端点应单独挂载，或使用未附加必需认证中间件的公开 blueprint。`AuthRequired` 只证明已认证；资源授权仍由应用负责。OpenAPI security 元数据需要独立声明实际使用的 cookie/header/Bearer 方案。

## 底层用法

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

可选的 `tools/openapi` 包根据最终路由元数据生成确定且经过校验的 OpenAPI 3.1 JSON：

```go
spec, err := openapi.Generate(routeList, openapi.Options{
	Title:   "Updater API",
	Version: "1.0.0",
})
```

每个生成的 operation 必须显式提供全局唯一的 `OperationID` 和至少一个响应。path 参数、请求体、响应体和 security 通过路由 option 声明。具名 `SchemaOf` 会生成稳定的 component。

应用类型的自定义 JSON 编码与反射结果不同时，使用 `JSONSchemaAlias() any` 描述 wire 类型。例如，编码为十进制 JSON string 的数量类型：

```go
func (ByteQuantity) JSONSchemaAlias() any { return "" }
```

该方法只描述 schema 类型，不改变 JSON 编码，也不执行运行时验证。生成的 schema 应以真实编码后的 payload 校验，尤其是使用自定义 marshaler 时。

响应头通过 `routes.Response.Headers` 声明，可以使用具名 schema：

```go
routes.Respond("204", routes.Response{
	Description: "Completed",
	Headers: map[string]routes.Header{
		"X-Request-ID": {Required: true, Schema: routes.SchemaOf[string]("")},
	},
})
```

响应头名称必须是合法 HTTP token，并且忽略大小写后唯一。`Content-Type` 通过 `Response.Content` 声明，不放入 `Headers`。响应头声明只描述契约；实际 header 仍由 handler 写入。

生成的 JSON 可交给任意兼容 OpenAPI 3.1 的 TypeScript 类型和 client 生成器；Kit 不内置 TypeScript 生成器。

handler 仍是普通 `net/http` handler。挂载路由不会做运行时 schema 校验；只有调用 `openapi.Generate` 时才生成并校验文档。

`routes.ExportJSON` 和 `routes.ExportMarkdown` 可用于生成精简路由摘要。OpenAPI 输出统一由 `openapi.Generate` 生成。
