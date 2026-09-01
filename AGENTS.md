# 代码风格

## Naming
Follow the global Naming rules. Only project-specific vocabulary and explicit overrides belong here.

### Golang Naming

Do not repeat package/receiver context: prefer `auth.Login`, `auth.Token`, `order.Create`; avoid `auth.AuthLogin`, `auth.AuthToken`, `order.CreateOrder`.

Avoid vague packages such as `common`, `utils`, `helper`, `misc`, `types`, `interfaces`; name packages by domain or capability.

Interfaces describe consumer-required behavior. Do not define interfaces only because an implementation exists. Avoid `I` prefixes and `Interface` suffixes.

Prefer `Name()` over `GetName()` for field-like access. Use `Get` only for external lookup, storage/cache read, RPC call, or similar operations.

Receiver names are short and consistent; do not use `this`, `self`, or long receiver names. Keep initialisms consistent: `ID`, `URL`, `HTTP`, `JSON`, `SQL`, `API`, `RPC`, `CPU`, `IO`, `FS`. Avoid all-caps constants such as `DEFAULT_TIMEOUT`.

## 注释

- 采用优质 Go 开源项目风格：默认写“是什么”，必要时补“怎么用”，按golang的文档工具风格注释。
- 注释服务于公开 API、调用者和维护者，不写教程，不写过程播报，不写自我宣传。
- 导出符号默认写注释；包注释保持极短。
- 对一眼能看出含义和调用方式的代码，不写注释。
- 对 IDE / 文档工具已能清楚提示用法的接口，不补重复注释。
- 需要写注释时，只写这些内容：做什么、如何调用、关键契约。
- 关键契约包括：是否允许 `nil`、是否会 panic、零值是否可用、是否返回副本、是否会修改输入、默认行为、错误语义、并发要求、必须配合的 API。
- 调用示例可以保留，但只保留最短可用示例，不展开解释。
- 内联注释只写 why、坑、兼容性约束和不明显的边界，不写显然流程。
- README 讲完整说明；源码注释不承担 README 职责。
- 英文在上，中文在下；两者内容对齐，不扩写，不重复解释。

## 单元测试
- 一眼可以在代码中发现的问题、太简单的逻辑，没必要写单元注释
- 最大限度克制单元测试编写，不要有事没事就写个单测。代码编写是高智商行为，不是幼儿园的活动。

# Git

## commit 描述

- 人工提交标题统一使用英文，格式固定为 `<type>: <summary>`，例如 `fix: reject invalid auth requirements`。
- 人工提交不使用 scope；写 `feat: add route contracts`，不要写 `feat(routes): add route contracts`。Dependabot 等自动生成的提交不受此限制。
- `<summary>` 使用小写开头的祈使句，描述提交完成后的结果，末尾不加句号。
- 提交类型必须准确，常用类型包括 `fix`、`feat`、`refactor`、`chore`、`docs`、`test`、`ci`、`style`、`release`。
- 单文件且标题已完整说明变化时可以省略正文。修改多个文件或包含多项实质变化时，必须使用英文正文：标题后空一行，以连续的 `- ` 列表逐项说明主要变化；每项使用小写开头的祈使句，末尾不加句号，列表项之间不插空行。
- 提交正文必须使用真实换行；不得把 `\n` 等转义文本写入 message。

## CHANGELOG
- 项目根目录使用 `CHANGELOG.md` 记录面向使用者的版本变化；格式按 tag 分节，如 `## v0.1.9 - 2026-05-19`。
- 每个版本只写使用者关心的公开变化，不写开发过程、自我说明、内部重构流水账。
- 常用分类为 `Breaking`、`Added`、`Changed`、`Fixed`、`Removed`、`Security`；没有内容的分类不要写。
- 破坏性变化必须进入 `Breaking`，包括公开 API、配置格式、持久化格式、Redis / DB key 或 schema、错误语义、默认行为、最低 Go 版本要求。
- README 只描述当前版本的正常用法；历史迁移、旧行为兼容影响、升级注意事项写入 `CHANGELOG.md` 和 release notes，不塞进普通使用文档。
- tag 注释和 GitHub Release 文案优先复用对应版本的 `CHANGELOG.md` 条目，避免同一版本出现两套说法。
