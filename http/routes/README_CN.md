# http/routes

`http/routes` 让路由元数据贴着 handler，并构建由 `chi` 驱动的标准 HTTP handler。

## 使用

使用 `routes.Blueprint` 声明和组合端点，再将最终路由列表交给 `routes.NewHandler`。`routes.Route` 也接受任意 `http.Handler`。应用需要管理已有 chi 路由器或直接汇总路由切片时，仍可使用 `routes.Mount` 和 `routes.Router`。

`Blueprint` 方法依次接收 `path`、`summary`、handler 函数或方法值，以及路由选项。

## 完整 HTTP handler

使用 `NewHandler` 构建标准 `http.Handler`，应用代码不需要创建 chi 路由器。先声明和组合路由，再将最终路径传入构造函数：

```go
root := routes.NewBlueprint()
root.Include("/api", api)

handler, err := routes.NewHandler(root.Routes(), routes.HandlerOptions{
	Middleware: []routes.Middleware{
		middleware.RequestID,
		middleware.AccessLog(middleware.AccessLogOptions{Logger: logger}),
		middleware.Recover(middleware.RecoverOptions{Logger: logger}),
		apiBoundary,
	},
	NotFound:         http.HandlerFunc(notFound),
	MethodNotAllowed: http.HandlerFunc(methodNotAllowed),
	Unauthorized:     http.HandlerFunc(unauthorized),
})
if err != nil {
	return err
}

server := &http.Server{
	Addr:              ":8080",
	Handler:           handler,
	ReadHeaderTimeout: 5 * time.Second,
}
```

`api`、`apiBoundary` 和失败响应 handler 由应用提供。构造函数创建一个扁平的 chi 路由器，复用 `MountWithOptions` 的校验、冲突检查和认证保护。无效声明按下文契约返回错误或 panic。不会自动安装任何中间件。

可选的 JSON 失败响应、覆盖预设和新增业务错误范例见 [http/response](../response/README_CN.md)。

| 配置 | 契约 |
|---|---|
| `Middleware` | 按声明顺序从外向内执行，反向退出。覆盖成功和未匹配请求，也覆盖空路由表；忽略 nil 项。 |
| `NotFound` | 接收未匹配的路径；nil 使用标准纯文本 404。 |
| `MethodNotAllowed` | 接收方法不匹配的请求；调用前根据匹配的已注册方法设置 Allow，按字母顺序排列。nil 返回无响应体的 405；不会从 GET 推断 HEAD 或 OPTIONS。 |
| `Unauthorized` | 与 `MountOptions.Unauthorized` 相同；nil 保留 JSON 401 保护。 |

在启动阶段构建 handler。后续修改传入的切片不会重新配置 handler；应用提供的 handler 和中间件本身必须支持并发请求。

请求边界策略放入 `HandlerOptions.Middleware`。仅针对 API 的策略应由应用中间件匹配 `/api` 或 `/api/` 前缀，再设置缓存响应头或应用 `LimitBody`。`DefaultMiddleware` 和 `IncludeMiddleware` 只包装端点，不覆盖路由匹配失败。请求体上限在读取时生效，不预读请求体，也不会自动将 404/405 替换为 413。

SPA 兜底应通过 `NotFound` 接入，不要在 API 端点旁注册全局 `/*`。应用的失败响应 handler 决定哪些路径返回 JSON 错误、静态文件或页面，使兜底不参与方法匹配和路由冲突检查。可复用中间件见 [http/middleware](../middleware/README_CN.md)。

## 路径

非空端点路径和路由前缀都以单个 `/` 开头，例如 `Get("/users", ...)` 配合 `MountAt(router, "/api")`。前缀不能以 `/` 结尾；不添加前缀时使用 `""`。两类参数都不接受首尾空白。

路径按 `prefix + path` 组合。空端点表示前缀本身，`"/"` 表示其带尾斜线的端点。组合后的路径不能为空：HTTP 根路由使用 `"/"`。

| 前缀 | 端点路径 | HTTP 路径 |
|---|---|---|
| `""` | `"/"` | `/` |
| `"/api"` | `""` | `/api` |
| `"/api"` | `"/"` | `/api/` |
| `"/api"` | `"/users"` | `/api/users` |
| `"/api"` | `"/users/"` | `/api/users/` |

`Route.Path` 使用同一套端点语法。`RoutesAt("/api")` 和 `WithPrefix("/api", ...)` 生成 `/api/users` 这样的路径，可以用空前缀直接挂载并导出文档。斜线或空白格式无效时，Blueprint 声明和组合方法会 panic，挂载和导出函数返回错误。允许声明空端点，但组合后的路径必须非空；挂载和导出也会拒绝空的最终路径。

完整的 chi 路由模式在挂载时检查；chi 的语法校验遇到 `/users/{id` 这样的错误模式会 panic。Blueprint 声明和组合方法，以及 `ExportJSON`、`ExportMarkdown` 摘要导出不校验完整路由模式。

同一前缀可以分多次挂载。Kit 按 HTTP 方法检查本批路由与目标 router 的 `Routes()` 快照；重复路由和通配符冲突会在写入本批路由前 panic，消息包含冲突路径。

- `/users/{id}/posts` 与 `/users/{name}/profile` 冲突：共享参数位置必须统一命名。
- `/files/*` 与 `/files/download`、`/files/{name}` 或 `/files/` 冲突；`/files` 可以单独注册。
- `/users/{id}` 与 `/users/me` 可以共存，静态路径优先；不同 HTTP 方法独立检查。

路径保持 chi 语法。正则和参数分隔符保留 chi 的分支规则；正则只按 chi 的锚点规则规范化，不推断不同表达式的等价性。

通过 Kit 注册应用端点才能执行这些检查。直接调用 chi 注册会绕过校验；chi 隐藏的 `Mount` 转发别名不在检测范围内。向现有子路由添加端点时，将该子路由传给 Kit。

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

handler 可以在 `*http.Request` 后接收最多 15 个动态 path 值。
最终挂载路由里的动态 path 名必须唯一。

参数值取自 `Request.PathValue`，按最终路径中的顺序传入，包含动态前缀和 `*`。路由中间件可以通过 `Request.SetPathValue` 覆盖参数，包括空字符串。

```go
func remote(w http.ResponseWriter, r *http.Request, remoteID string) {
	_ = remoteID
}
```

`Blueprint.Routes()` 和 `RoutesAt()` 返回拥有所有权的 `[]routes.Route` 副本。挂载和契约生成应使用同一组最终路由。动态前缀参数默认为必填的 string path 参数；显式元数据优先。

在启动阶段完成 Blueprint 构建和挂载，开始服务后不再修改路由或中间件配置。同一个 Blueprint 可以挂载到不同前缀，每次挂载的参数绑定相互独立。异步任务应显式接收需要的参数值，不要在路由返回后访问 chi 的路由 context 或继续使用 `ResponseWriter`。

`AuthPublic`、`AuthOptional` 和 `AuthRequired` 会导出为路由元数据。`Mount` 也会在运行时保护 `AuthRequired` 路由，所以认证中间件必须在认证成功后调用 `routes.WithAuthenticated` 标记请求。

## 应用认证接入

认证中间件可以使用应用自己的 session 和 principal，也可以使用可选的 `auth/http`、`auth/rbac/http` 包。资源授权和会话生命周期由应用负责。

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

这里的 `sessions`、`withPrincipal` 和响应 handler 由应用提供。`MountOptions.Unauthorized` 处理必需认证路由缺少认证标记的情况，不能继续执行端点，也不能覆盖认证中间件已经写出的响应。Nil 使用默认 JSON 401 响应。

`AuthPublic` 和 `AuthOptional` 不会移除已经附加的中间件。登录端点应单独挂载，或使用未附加必需认证中间件的公开 blueprint。`AuthRequired` 只证明已认证；资源授权仍由应用负责。OpenAPI security 元数据需要独立声明实际使用的 cookie/header/Bearer 方案。

## 路由切片

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

可选的 `tools/openapi` 包根据最终路由元数据生成确定且经过校验的 OpenAPI 3.1 JSON，不在运行时校验 HTTP 请求或响应。

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

生成的 JSON 可交给兼容 OpenAPI 3.1 的 TypeScript 类型和 client 生成器。精简路由摘要使用 `routes.ExportJSON` 或 `routes.ExportMarkdown`。
