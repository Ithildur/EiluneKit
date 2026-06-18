# auth/http

`auth/http` 保存认证包的 HTTP 适配层。

admin-only、单用户或共享口令应用使用 `auth/http/basic`，它提供 cookie refresh session、CSRF 保护，以及可选的 bearer / API key 中间件。尽管包名是 basic，它不是 RFC 7617 HTTP Basic authentication。

多用户 JSON bearer 认证使用 `auth/http/rbac`，它提供 login、refresh、logout、当前主体、角色中间件、scope 中间件，以及可选 opaque API token。
