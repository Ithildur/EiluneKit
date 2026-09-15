# http/static

`http/static` 提供项目内静态目录和 SPA handler。导入路径：`github.com/Ithildur/EiluneKit/http/static`；包名：`static`。

## 快速开始

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

这里的 `api` 是应用的 Blueprint；将 `handler` 交给 `http.Server.Handler`。应用选择为 `/api` 和 `/api/…` 返回 JSON 404，其余路径交给 SPA。History 回退仅用于不存在的路径；其他文件系统错误使用文件服务器的错误响应。

与 API 共用路由器时，使用兜底注入。全局 `/*` 会参与方法匹配，并与下级路由冲突，可能接走本应进入 API 404/405 handler 的请求。

已有的 chi 集成仍可显式挂载，但该前缀应由静态 handler 独占：

```go
if _, err := static.MountSPA(r, "/app", "dist", static.Options{
	Development: true,
}); err != nil {
	return err
}
```

## 解析规则

- `relPath` 必须是干净的项目内相对路径，例如 `dist` 或 `web/dist`
- 绝对路径、`.`、`..`、重复分隔符和脏路径都会以 `ErrInvalidProjectPath` 拒绝
- SPA 解析要求存在 `index.html`
- 当 `Options.Development` 为 `false` 时，只使用应用目录环境变量和可执行文件目录进行发现
- 当 `Options.Development` 为 `true` 时，额外允许工作目录作为发现来源

## 选项

- `AppDir`：透传给 `appdir.DiscoverHome`
- `Development`：启用本地开发时的工作目录回退

## 契约

- `SPAHandler`、`MountSPA`、`Mount`、`ResolveDir` 和 `ResolveSPADir` 都接收一个 `Options` 结构体；需要默认行为时传入 `static.Options{}`
- 当 `Options.AppDir.Markers` 为空时，包会根据 `relPath` 自动推导 markers
- 非法的应用目录环境变量覆盖会直接以 `appdir.ErrEnvInvalid` 失败，不会再回退到工作目录
