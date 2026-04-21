# http/static

`http/static` 用于在 `chi` 上提供项目内静态目录和 SPA 资源。导入路径：`github.com/Ithildur/EiluneKit/http/static`；包名：`static`。

## 快速开始

```go
handler, err := static.SPAHandler("dist", static.Options{
	AppDir: appdir.Options{EnvVar: "APP_HOME"},
})
if err != nil {
	return err
}

r.Handle("/*", handler)
```

显式挂载：

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
