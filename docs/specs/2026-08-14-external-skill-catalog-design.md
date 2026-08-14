# 外置 Skill 注册与 easy-cli 聚合索引设计

## 1. 背景与目标

当前 easy CLI 只从二进制内嵌的 `skills/*/SKILL.md` 发现 skill。业务约束较多时，把所有内容继续维护在仓库内置 skill 中会导致聚合入口过度绑定具体业务；如果把外置 skill 的正文全部拼进 `easy-cli`，又会造成 prompt 膨胀、内容重复和更新滞后。

本次目标是增加外置 skill 的本地发现能力，并让 `easy-cli` 成为轻量的统一入口：

1. 自动读取用户级和项目级配置目录下的外置 skill。
2. 将外置 skill 合并进运行时 Registry，使 `list`、`show`、`prompt` 和快捷命令都能使用它们。
3. 执行 `easy skill install easy-cli` 或 `easy skill update easy-cli` 时，只把外置 skill 的元数据索引写入已安装的 `easy-cli`，不复制正文。
4. 让 AI 先读取聚合索引，再通过 `easy skill prompt <name>` 按需加载单个 skill。
5. 保持现有内置 skill、安装路径和普通 CLI 使用方式兼容。

## 2. 非目标

- 第一阶段不把外置 skill 的正文拼接到 `easy-cli`。
- 第一阶段不支持外置 skill 的多文件资源包、`references/` 或资源子命令。
- 第一阶段不支持从 Git、HTTP、飞书或其他远程来源导入 skill。
- 第一阶段不增加模型摘要、向量检索或网络搜索。
- 第一阶段不提供从任意路径复制 skill 到配置目录的 `import` 命令；“导入”定义为按约定目录自动发现。
- 不修改 Codex 的全局配置文件。
- 不让外置 skill 自动安装到 `.agents/skills`；只有用户显式执行 `easy skill install <name>` 时才安装。

## 3. 关联模块

- `cmd/easy`
- `internal/cli`
- `internal/skill`
- `internal/projectroot`
- `skills`
- `skills/easy-cli`
- `README.md`

模块与 spec 的双向索引维护在 `docs/specs/index.md`。

## 4. 外置 Skill 目录与发现

### 4.1 目录约定

用户级外置 skill 位于：

```text
<home>/.config/easy-cli/skills/<name>/SKILL.md
```

项目级外置 skill 位于：

```text
<project-root>/.easy-cli/skills/<name>/SKILL.md
```

其中 `<home>` 使用 CLI 的 `HomeDir` 选项或运行时用户 Home，`<project-root>` 沿用 `projectroot.Find` 的 Git 根目录发现规则。

每个外置 skill 必须使用独立目录，目录名必须与 `SKILL.md` front matter 中的 `name` 一致。只扫描 `skills` 目录的直接子目录，不递归扫描任意深度文件。

不存在的外置目录视为空，不创建目录；目录存在但包含格式错误的 skill 时，当前命令失败并报告具体文件路径和原因。

### 4.2 来源优先级

运行时按以下顺序加载并合并：

```text
内置 skill < Home 外置 skill < 项目外置 skill
```

同名 skill 由优先级更高的来源覆盖。覆盖后的 skill 仍保留来源信息，供 `show`、JSON 输出和聚合索引使用。这样可以让项目或用户对现有业务 skill 做本地替换，而不修改二进制内置内容。

`easy-cli` 是聚合入口名称，属于保留名称。外置目录不得声明 `name: easy-cli`，避免外部正文覆盖聚合入口；其他内置 skill 名称允许被 Home 或项目 skill 覆盖。

### 4.3 Skill 数据模型

在现有 `Skill` 元数据基础上增加来源信息：

```text
Name
Description
Source       原始 SKILL.md 内容
Body
Origin       builtin | home | project
SourcePath   内置使用嵌入路径，外置使用绝对文件路径
```

来源信息不改变现有 prompt 正文；它只用于诊断、索引生成和 JSON 元数据输出。

## 5. Registry 与 CLI 行为

### 5.1 Registry 加载边界

保留现有 `skill.Load(fsys)`，用于测试和只加载指定文件系统的场景；增加组合加载能力，由 `cmd/easy` 启动流程调用：

```text
skill.LoadAll(embeddedFS, DiscoveryOptions{WorkingDir, HomeDir})
```

`LoadAll` 负责：

1. 加载内置嵌入 skill。
2. 读取 Home 外置目录。
3. 读取项目外置目录。
4. 按来源优先级合并。
5. 校验名称、目录和保留名称。

CLI 继续接收已构建的 Registry，不在每个子命令中重复读取文件系统。外置 skill 因此自动支持：

```bash
easy skill list
easy skill show <name>
easy skill prompt <name>
easy <name>
```

### 5.2 输出兼容性

`easy skill list` 继续输出名称、描述和安装状态三列，但数据范围扩展为合并后的全部 skill。

`easy skill show <name>` 在现有元数据和安装状态之外增加来源信息，至少包含 `Origin`；外置 skill 的绝对路径可用于诊断，但不得输出 skill 正文。

`easy skill prompt <name> --format json` 增加 `origin` 和 `source_path` 字段；原有 `name`、`description`、`raw` 和 `prompt` 字段保持不变。

普通文本 prompt 只输出 skill 正文，不混入来源和状态信息。

### 5.3 安装和更新

普通 skill 的 `install` 与 `update` 继续使用合并后 Registry 中选中的 skill，因此外置 skill 可以被显式安装到 `.agents/skills`。

`easy-cli` 具有特殊的聚合渲染流程：

1. 读取内置 `skills/easy-cli/SKILL.md` 作为稳定基础内容。
2. 找到预留的生成区块。
3. 根据合并后的 Registry 生成外置 skill 索引。
4. 只写入每个外置 skill 的名称、描述、来源和加载命令。
5. 保留所有 skill 正文在其原始来源中。

`install easy-cli` 和 `update easy-cli` 都执行上述渲染。`update` 仍要求目标文件已经存在；不存在时沿用现有错误并提示先安装。

## 6. easy-cli 聚合索引

### 6.1 生成区块

`skills/easy-cli/SKILL.md` 增加稳定的生成标记，例如：

```markdown
<!-- EASY_EXTERNAL_SKILLS_START -->
<!-- EASY_EXTERNAL_SKILLS_END -->
```

生成内容采用紧凑 Markdown：

```markdown
## External skills

- `smb-work-order` — SMB 本地开发约束（project）
  Load with: `easy skill prompt smb-work-order`
```

只索引 `Origin != builtin` 的 skill；内置能力继续由 `easy-cli` 的静态路由说明维护，避免重复生成。没有外置 skill 时生成明确的空状态说明。

### 6.2 按需加载原则

聚合 skill 只负责告诉 AI：

- 有哪些外置能力；
- 每项能力的一句话描述；
- 应该使用哪个 `easy skill prompt <name>` 命令加载正文。

AI 不需要因为加载 `easy-cli` 而读取所有外置正文。新增外置 skill 后，显式执行 `easy skill update easy-cli` 即可刷新已安装聚合入口；运行时的 `list`、`show` 和 `prompt` 使用当前目录内容。

## 7. 错误处理与安全

- Home 或项目外置目录不存在时不报错。
- 外置 `SKILL.md` 缺少 front matter、`name`、`description` 或目录名不匹配时返回业务错误，并包含文件路径。
- 外置 skill 声明保留名称 `easy-cli` 时拒绝加载。
- 同一来源中出现无法解析的外置 skill 时，不生成部分索引，避免聚合入口静默缺失能力。
- 用户输入的 skill 名称仍必须经过现有名称校验，不把名称拼接为任意源路径。
- 外置 skill 只作为 Markdown 指令读取，不执行其中的脚本、命令或网络请求。
- 不输出凭据、配置密码或 skill 正文到错误信息。
- 聚合索引的描述应按单行文本渲染，避免外置 description 注入额外 Markdown 结构。

## 8. 测试策略

遵循 TDD，先为以下行为写失败测试：

### Registry 测试

- Home 和项目目录中的 skill 能被发现。
- 不存在的目录被视为空。
- 项目 skill 覆盖同名 Home skill。
- Home skill 覆盖同名内置 skill，但 `easy-cli` 外置覆盖被拒绝。
- 目录名与 front matter 名称不一致时失败。
- 外置 skill 的来源和路径被保留。
- 内置 Registry 的现有 `Load` 行为不变。

### CLI 测试

- 合并后的外置 skill 可通过 `list`、`show`、`prompt` 和快捷命令访问。
- `show` 和 JSON prompt 输出来源信息。
- `easy-cli` 的 prompt 只包含外置 skill 索引，不包含外置正文。
- `install easy-cli` 生成带当前外置索引的文件。
- `update easy-cli` 刷新已安装文件中的索引。
- `update easy-cli` 在外置目录变化后只改变索引区块。
- 普通 skill 的安装、更新和现有幂等/冲突行为保持不变。

### 回归验证

- `go test ./...`。
- `go vet ./...`。
- `go build ./cmd/easy`。
- 临时 Home、项目目录下的手工 CLI 流程验证。

## 9. 验收标准

1. 在 `~/.config/easy-cli/skills/<name>/SKILL.md` 放置合法 skill 后，`easy skill list` 能发现它。
2. 在项目 `.easy-cli/skills/<name>/SKILL.md` 放置同名 skill 后，项目版本覆盖 Home 版本。
3. `easy skill prompt <name>` 能按需输出外置 skill 的压缩正文。
4. `easy skill update easy-cli` 后，已安装 `easy-cli` 能看到外置 skill 的名称、描述和加载命令。
5. 聚合 `easy-cli` 不包含外置 skill 正文，且正文变化不会导致索引内容膨胀。
6. 外置 skill 能通过显式 `easy skill install <name>` 安装，普通命令不会隐式安装。
7. 非法 skill、保留名称和路径冲突都有明确错误，不会生成半成品聚合文件。
8. 现有内置 skill、快捷命令、项目级/全局级安装和更新测试全部通过。

## 10. 后续扩展

当单个 skill 需要大量参考资料时，再设计多文件 skill 资源包和按文件读取命令；当 skill 数量明显增加时，再增加基于 `name`、`description` 和可选标签的 `easy skill search`。这些能力不进入本次实现。
