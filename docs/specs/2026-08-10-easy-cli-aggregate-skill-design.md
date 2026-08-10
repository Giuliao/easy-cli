# easy-cli 聚合 Skill 与使用路由设计

## 1. 背景与目标

当前仓库中的每个 skill 可以通过 `easy skill prompt <name>` 输出压缩后的 prompt，也可以通过同名快捷命令输出。随着内置能力增加，仅依靠独立 skill 的描述，Agent 可能把“使用某个能力”误判成“安装某个 skill”。

本次目标是新增一个名为 `easy-cli` 的聚合 skill，使 Agent 在加载它后能够：

1. 识别 easy CLI 的整体能力和命令结构。
2. 根据用户任务选择并调用其他 skill 或直接调用 CLI 功能。
3. 将 `install`、`update` 视为显式的 skill 管理操作，不作为普通任务的默认步骤。
4. 通过 `easy skill install easy-cli` 和 `easy skill update easy-cli` 安装、更新这个聚合 skill。

聚合 skill 的内容在仓库中维护，并随 Go 二进制通过 `embed` 发布。

## 2. 非目标

- 不把 `easy-cli` 安装时自动转换为所有子 skill 的安装操作。
- 不在 Agent 未明确要求时执行 skill 安装或更新。
- 不让聚合 skill 替代子 skill 的详细业务约束；它只负责路由、用法和能力索引。
- 不通过网络动态生成或更新聚合 skill。
- 不新增 spec 管理、索引生成或其他项目开发约定相关的 CLI 功能。

## 3. 关联模块

- `cmd/easy`
- `internal/cli`
- `internal/skill`
- `skills`
- `skills/easy-cli`
- `skills/mysql-ddl-export`
- `skills/smb-work-order`

模块与 spec 的双向索引维护在 `docs/specs/index.md`。

## 4. Agent 使用路由

`easy-cli` 的正文必须将“使用能力”和“管理 skill”明确分开，并把使用能力放在前面。

### 4.1 默认规则

聚合 skill 的首要规则如下：

> 当用户请求某项能力时，优先通过 easy CLI 调用对应 skill 或功能。不要因为识别到某个 skill 就执行 `install` 或 `update`。只有用户明确要求安装、更新或管理 skill 时，才执行对应管理命令。

Agent 使用其他 skill 时，应优先执行以下命令取得压缩后的 prompt，并将命令输出作为当前任务的约束上下文：

    easy skill prompt <skill-name>

同名快捷命令可以作为等价简写：

    easy <skill-name>

当需要发现能力或确认 skill 描述时，使用：

    easy skill list
    easy skill show <skill-name>

### 4.2 当前能力路由

聚合 skill 至少包含以下能力映射：

| 用户意图 | 默认命令 | 说明 |
| --- | --- | --- |
| 发现 easy CLI 能力 | `easy skill list` | 查看内置 skill 名称、描述和安装状态 |
| 获取 SMB 小工单开发约束 | `easy skill prompt smb-work-order` | 读取压缩后的项目开发与交付规则 |
| 获取 MySQL DDL 导出工作流 | `easy skill prompt mysql-ddl-export` | 读取连接确认、密码保护和结果处理规则 |
| 实际导出 MySQL 表 DDL | `easy mysql ddl ...` | 直接执行只读的 MySQL 普通表 DDL 导出 |
| 查看单个 skill 元数据 | `easy skill show <name>` | 查看描述和安装状态 |

对于 `mysql-ddl-export`，Agent 需要先使用 skill prompt 获取工作流；实际访问数据库时再执行 `easy mysql ddl`，不能把读取 prompt 误当成已经完成 DDL 导出。

### 4.3 管理操作边界

以下命令只在用户明确提出“安装”“更新”“管理 skill”等意图时使用：

    easy skill install <name>
    easy skill install <name> --global
    easy skill install <name> --force
    easy skill update <name>
    easy skill update <name> --global

普通开发、DDL 查询或项目约束读取任务不得自动调用上述命令。安装 `easy-cli` 只安装聚合 skill 本身，不安装 `mysql-ddl-export` 或 `smb-work-order`。

## 5. 聚合 Skill 内容与维护方式

新增文件：

    skills/easy-cli/
    ├── SKILL.md
    └── agents/openai.yaml

`SKILL.md` 使用标准 front matter：

    ---
    name: easy-cli
    description: Use the easy CLI to discover and invoke repository skills and built-in capabilities; use installation or update commands only when the user explicitly asks to manage skills.
    ---

正文采用渐进式结构，顺序固定为：

1. 使用规则和“不要默认安装”的边界。
2. CLI 发现、读取和调用 skill 的通用流程。
3. 当前内置 skill 和直接功能命令的能力映射。
4. MySQL DDL 命令参数、安全要求和输出边界的简要入口。
5. skill 安装与更新的管理命令。

聚合 skill 采用仓库内静态维护，不在运行时拼接 help 输出。新增 CLI 功能或内置 skill 时，必须同步更新 `skills/easy-cli/SKILL.md`、对应测试和 README 中的用法说明。

## 6. 安装与更新行为

### 6.1 安装

现有通用安装命令支持聚合 skill：

    easy skill install easy-cli
    easy skill install easy-cli --global

项目级目标为项目根目录的 `.agents/skills/easy-cli/SKILL.md`，用户级目标为 `~/.agents/skills/easy-cli/SKILL.md`。安装内容仍使用本地 prompt 压缩器处理，目标内容相同则幂等成功，目标内容不同则遵循现有冲突保护，只有 `--force` 才覆盖。

### 6.2 更新

新增命令：

    easy skill update <name>
    easy skill update <name> --global

更新从当前二进制内置 registry 读取最新 source，并覆盖对应已安装文件。更新只接受已存在的安装目标；目标不存在时返回错误并提示先执行 `install`。更新不需要额外的 `--force`，覆盖行为由 `update` 的语义授权。

`update` 支持所有已注册 skill，但不会批量更新，也不会自动安装其他 skill。更新输出应明确区分 `Updated`、目标不存在和源内容无变化等情况。

## 7. CLI 与内部边界

- `skills/embed.go` 继续通过 `*/SKILL.md` 自动嵌入新增聚合 skill。
- `internal/cli` 新增 `update` 子命令路由、参数解析、帮助文本和退出码。
- `internal/skill` 提供复用现有安装路径、压缩和原子写入逻辑的更新能力，并保持项目级、用户级目标计算一致。
- `skills/embed_test.go` 验证 `easy-cli` 被嵌入、可解析且包含路由规则。
- `internal/cli` 测试验证 install/update 的项目级和全局行为、冲突与缺失目标错误，以及普通使用流程不会触发安装。

标准输出约束保持不变：prompt 和 DDL 内容写入 stdout；状态、错误和管理操作结果写入可测试的输出流，不把凭据或完整 DSN 写入输出。

## 8. 验收标准

1. `easy skill list` 能看到 `easy-cli` 及其描述。
2. `easy skill prompt easy-cli` 的内容明确指导 Agent 使用 `easy skill prompt <name>` 或 `easy <name>`，并明确禁止默认安装。
3. Agent 处理 SMB 约束任务时，聚合 skill 指向 `easy smb-work-order` 或对应 prompt，而不是 `easy skill install smb-work-order`。
4. Agent 处理 MySQL DDL 任务时，聚合 skill 指向 `easy mysql ddl`，并保留密码和只读安全边界。
5. `easy skill install easy-cli`、`--global` 和 `--force` 行为可用。
6. `easy skill update easy-cli` 和 `--global` 能更新已安装版本，未安装时给出可操作错误。
7. `make check`、skill quick validation 和手工 CLI 验证全部通过。
