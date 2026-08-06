# easy CLI Skill 管理工具设计

## 1. 背景与目标

easy 是一个基于 Go 的 CLI 工具，用于把日常开发中维护在飞书文档里的 prompt/skill 纳入代码仓库，并以可复用、可安装、可被 AI 识别的方式提供给开发者。

第一版目标：

1. 在仓库内维护 skill 内容。
2. 通过 CLI 子命令列出、查看、输出和安装 skill。
3. 对 prompt 做确定性的本地结构压缩，减少冗余但不依赖模型和网络。
4. 支持项目级和用户全局级安装。
5. 通过动态快捷子命令让每个 skill 与 CLI 命令一一对应。

第一版内置 skill 为 `smb-work-order`，内容来源于用户提供的飞书文档“SMB 小工单项目本地开发约束”。飞书文档只作为迁移来源，运行时不再依赖飞书。

## 2. 非目标

- 第一版不调用大模型进行摘要或改写。
- 第一版不从 Git、HTTP 或飞书远程安装 skill。
- 第一版不提供 skill 在线市场、版本解析或依赖管理。
- 第一版不修改用户的 Codex 配置文件。
- 第一版不尝试推断用户没有写出的业务规则。

## 3. 关键用户流程

### 查看和输出

```bash
easy skill list
easy skill show smb-work-order
easy skill prompt smb-work-order
easy smb-work-order
```

`easy skill prompt` 和快捷命令默认只把压缩后的 prompt 写到 stdout，诊断信息写到 stderr，因此可以安全地管道到其他工具：

```bash
easy skill prompt smb-work-order | pbcopy
```

可选输出：

```bash
easy skill prompt smb-work-order --raw
easy skill prompt smb-work-order --format json
```

### 安装

```bash
easy skill install smb-work-order
easy skill install smb-work-order --global
easy skill install smb-work-order --force
```

项目级安装目标为当前项目根目录下的 `.agents/skills/<name>/SKILL.md`；用户级安装目标为 `~/.agents/skills/<name>/SKILL.md`。项目根目录优先通过 Git 根目录确定；当前目录不在 Git 仓库内时使用当前工作目录。

安装输出包含 skill 名称、安装目标和结果。目标文件内容相同视为幂等成功；内容不同则默认拒绝覆盖，只有显式传入 `--force` 才覆盖。

## 4. Skill 数据模型与仓库布局

仓库布局：

```text
easy-cli/
├── cmd/easy/main.go
├── internal/
│   ├── cli/
│   ├── prompt/
│   └── skill/
├── skills/
│   └── smb-work-order/
│       └── SKILL.md
├── docs/superpowers/specs/
├── go.mod
└── README.md
```

每个内置 skill 都是一个目录，并包含符合 Agent Skills 约定的 `SKILL.md`：

```markdown
---
name: smb-work-order
description: SMB 小工单项目的本地开发约束与交付规范。
---

# SMB 小工单项目本地开发约束

...
```

`name` 是唯一标识，同时是动态快捷子命令的名称；`description` 用于列表展示以及安装后供 AI 进行 skill 匹配。正文是仓库内可审阅、可版本化的 prompt 来源。

使用 Go `embed` 将 `skills/*` 内置到二进制。运行时从嵌入文件系统加载并解析 skill，不需要网络访问。

核心内部边界：

- `skill`：解析 front matter、发现内置 skill、生成安装内容、计算安装状态。
- `prompt`：只负责 Markdown 的确定性结构压缩。
- `cli`：负责参数解析、命令路由、退出码和 stdout/stderr 输出。

## 5. 命令设计

管理命令：

| 命令 | 行为 |
| --- | --- |
| `easy skill list` | 列出所有内置 skill 的名称、描述和安装状态 |
| `easy skill show <name>` | 显示 skill 元数据、源内容摘要和安装状态 |
| `easy skill prompt <name>` | 输出压缩后的 prompt |
| `easy skill prompt <name> --raw` | 输出原始正文 |
| `easy skill prompt <name> --format json` | 输出结构化 JSON |
| `easy skill install <name>` | 安装到项目级 `.agents/skills` |
| `easy skill install <name> --global` | 安装到用户级 `~/.agents/skills` |
| `easy skill install <name> --force` | 允许覆盖不同内容的目标文件 |

快捷命令：

```bash
easy <skill-name>
```

快捷命令等价于 `easy skill prompt <skill-name>`。内置管理命令优先于 skill 名称，避免 skill 名称覆盖 `skill`、`help` 等保留命令。未知命令展示帮助和可用 skill 列表。

动态命令来自嵌入的 skill 注册表，不需要为新增 skill 修改 Go 命令分发代码。

## 6. Prompt 压缩算法

压缩器的原则是“保留语义、压缩结构”，不进行模型式摘要。

处理步骤：

1. 解析 YAML front matter，并保留 `name`、`description`。
2. 规范化换行符、行尾空格和列表缩进。
3. 删除与 front matter `name` 重复的顶层标题。
4. 识别标准 Markdown 表格，将表头和数据行转换为键值列表；多列数据使用紧凑的同一列表项表达。
5. 合并连续空行，保留标题、列表、代码块和段落边界。
6. 删除连续出现的完全重复行或重复块。
7. 不删除业务句子，不做同义词替换，不补充缺失事实，不改变代码块内容。

输出仍为结构化 Markdown，使 AI 可以稳定识别任务目标、上下文、约束、流程和输出要求。

压缩算法需要对 fenced code block 做状态跟踪，表格转换不能误处理代码块中的管道符或普通文本中的管道符。

## 7. 安装与文件安全

安装器生成目标目录下的 `SKILL.md`，正文使用默认压缩结果，front matter 使用原始元数据。写入采用同目录临时文件加原子 rename，避免进程中断时留下半截文件。

安装器必须：

- 拒绝空名称、路径穿越和非法 skill 名称。
- 只接受由内置注册表解析出的 skill 名称，不把用户输入拼接为任意源路径。
- 在覆盖前比较文件内容；相同内容不重复写入。
- 目标目录创建失败或写入失败时不报告成功。
- 不输出任何凭据或环境变量。

## 8. 错误与退出码

统一退出码：

- `0`：成功。
- `1`：skill 不存在、文件格式错误、安装失败或其他业务错误。
- `2`：命令用法错误、参数缺失或参数冲突。

错误文本写入 stderr。prompt 的纯文本 stdout 不混入状态信息，JSON 模式仅在成功时写入完整 JSON。

需要覆盖的典型错误：未知 skill、缺少 front matter、缺少 `name`/`description`、目标文件冲突、目标目录不可写、非法 `--format` 和非法选项组合。

## 9. 测试策略

采用 TDD 的 Red-Green-Refactor 循环，先写失败测试，再实现最小行为。

单元测试：

- front matter 解析与必填字段校验。
- 嵌入 skill 注册表加载。
- Markdown 表格转换。
- 空行、行尾空格和重复内容压缩。
- 代码块保护。
- raw、压缩和 JSON 输出数据。
- 项目级与全局级目标路径计算。
- 相同内容幂等安装。
- 冲突拒绝覆盖与 `--force` 覆盖。

集成测试：

- `easy skill list`、`show`、`prompt`、`install`。
- 动态 `easy smb-work-order` 快捷命令。
- 未知命令和未知 skill 的退出码与 stderr。
- 在临时目录中执行完整安装流程。

验收标准：

1. 新增一个 `skills/<name>/SKILL.md` 后，重新编译即可出现在 `easy skill list` 中。
2. `easy skill prompt smb-work-order` 输出稳定、无诊断噪声且比原始内容更紧凑。
3. 项目级和全局级安装均生成可被 Codex 识别的 `SKILL.md`。
4. 重复安装幂等，覆盖行为必须显式确认。
5. `go test ./...` 和构建命令通过。

## 10. 后续扩展边界

后续可在不破坏当前命令契约的前提下增加：

- 从本地目录或 Git 仓库安装。
- skill 版本和更新检查。
- 更多内置 skill。
- 可插拔压缩策略。
- 其他 AI 工具的安装适配器。

这些能力不进入第一版实现。
