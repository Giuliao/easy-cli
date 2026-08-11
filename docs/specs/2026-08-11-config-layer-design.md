# easy CLI 配置与 Home 初始化设计

## 1. 背景与目标

`easy mysql ddl` 和 `easy mysql query` 当前要求每次都在命令行提供完整的连接参数；`smb-work-order` skill 则把开发者本机的 SMB 三个仓库绝对路径写死在内置 prompt 中。这两种方式都不适合在不同项目和不同开发者之间复用。

本次为 easy CLI 增加本地配置层，目标如下：

1. 从 Home 和当前项目两个位置读取 JSON 配置。
2. 项目配置按字段覆盖 Home 配置；命令行显式参数继续拥有最高优先级。
3. 让 MySQL DDL 与 query 命令能使用配置中的连接信息，包括密码。
4. 移除 SMB skill 中写死的个人仓库绝对路径，改由 CLI 解析合并后的 SMB 配置。
5. 提供只读、非敏感的配置查询命令，供 skill 在运行时取得 SMB 仓库位置。
6. 不将密码输出到 stdout、stderr、帮助文本、skill 或版本库。
7. 提供显式命令在 Home 目录安全地初始化可编辑的配置模板。

## 2. 非目标

- 不实现交互式配置编辑、配置同步、远程分发或配置 profile。
- 不增加环境变量覆盖、配置热更新或配置迁移。
- 不让 `easy config get` 输出 MySQL 密码或任意未声明字段。
- 不改变 `mysql query` 对 `--sql` 的必填要求，也不限制其 SQL 类型。
- 不在找不到配置文件时自动猜测数据库、账号或 SMB 仓库路径。
- 不把该配置层用于 skill 安装目录、spec 工作流或其他无关能力。

## 3. 关联模块

- `cmd/easy`
- `internal/config`
- `internal/projectroot`
- `internal/cli`
- `internal/mysql`
- `skills/easy-cli`
- `skills/mysql-ddl-export`
- `skills/smb-work-order`
- `README.md`
- `.gitignore`

模块与 spec 的双向索引维护在 `docs/specs/index.md`。

## 4. 配置文件位置与格式

每次执行 easy 命令时，按以下顺序读取存在的文件：

1. Home 配置：`~/.config/easy-cli/config.json`。
2. 项目配置：`<项目根目录>/.easy-cli/config.json`。

Home 目录通过运行时的 `os.UserHomeDir` 取得；不能在代码或 skill 中写死某个用户目录。项目根目录沿用 skill 安装时的规则：从当前工作目录向上查找 `.git`，找不到 Git 仓库时使用当前工作目录本身。

Home 配置由 `easy config init` 显式初始化；普通读取、skill 命令和帮助不会自动创建该文件。

配置使用 JSON，示例：

```json
{
  "mysql": {
    "host": "127.0.0.1",
    "port": 3306,
    "user": "readonly",
    "password": "local-secret",
    "database": "app"
  },
  "smb": {
    "backend_repo": "/Users/name/CodeHub/smb_application",
    "frontend_repo": "/Users/name/CodeHub/lark_smb_work_order_fe",
    "idl_repo": "/Users/name/CodeHub/lark_smb_agent_idl"
  }
}
```

仓库提交 `.easy-cli/config.example.json` 作为无密示例，真实项目配置 `/.easy-cli/config.json` 必须被 `.gitignore` 忽略。Home 配置是用户本机文件，不纳入仓库。文档说明应建议将含密码的 Home 配置设为仅当前用户可读。

## 5. 加载、合并与校验

### 5.1 加载规则

- 两个配置文件都不存在时，加载结果为空，现有命令仍可完全通过显式参数使用。
- Home 配置先加载，项目配置后加载。
- 文件存在但不可读、JSON 语法非法、字段类型不正确或含有未知字段时，当前命令失败并在 stderr 报告配置文件路径和原因。
- 读取、解析和错误包装不得包含 `mysql.password` 的值。
- 配置不缓存；每次 CLI 进程运行时重新加载一次。

### 5.2 字段级合并

配置模型内部须保留“字段未声明”与“字段显式为空/零值”的区别。项目配置只覆盖其中实际声明的字段；未声明的字段继续继承 Home 配置，不能因 Go 零值覆盖掉 Home 值。

合并后的 MySQL 连接字段为 `host`、`port`、`user`、`password` 和 `database`；SMB 字段为 `backend_repo`、`frontend_repo` 和 `idl_repo`。

### 5.3 MySQL 参数优先级

最终 MySQL 连接参数依以下顺序确定：

```text
命令行显式参数 > 项目配置 > Home 配置 > 内置默认值
```

内置默认值仅包括 MySQL 端口 `3306`。`--password` 与 `--password-stdin` 均视为命令行显式密码来源，且继续互斥；未提供这两个参数时可以使用配置密码。`--password-stdin` 读取到的密码覆盖任何配置密码。

命令行解析必须记录每个连接 flag 是否显式提供，不能在解析开始时就用默认端口或空值覆盖配置。完成合并后再校验 host、user、password、database 与端口范围。缺失字段的报错应说明可通过对应 flag 或配置补充。

`mysql query` 的 `--sql`、`--format` 保持独立于配置：`--sql` 必填，`--format` 仍默认 `json`。

## 6. 配置命令

### 6.1 初始化 Home 配置

新增命令：

```text
easy config init [--force]
```

该命令只操作 `~/.config/easy-cli/config.json`，不读取、创建或修改当前项目的 `.easy-cli/config.json`。首次执行时创建父目录（权限 `0700`）和配置文件（权限 `0600`），并写入以下无密模板：

```json
{
  "mysql": {
    "host": "",
    "port": 3306,
    "user": "",
    "password": "",
    "database": ""
  },
  "smb": {
    "backend_repo": "",
    "frontend_repo": "",
    "idl_repo": ""
  }
}
```

若目标文件已存在，命令以退出码 `1` 失败且不修改原文件，并提示使用 `--force`。`--force` 以原子替换方式覆盖目标文件，最终权限仍为 `0600`。只接受 `--force`、`--help` 和 `-h`；未知参数或重复参数为退出码 `2` 的参数错误。初始化模板不含真实凭据，且不输出配置内容。

### 6.2 查询配置

新增命令：

```text
easy config get <key>
```

它使用同一套 Home/项目加载和合并规则，将单个值加换行写到 stdout。支持的键为：

| 键 | 用途 |
| --- | --- |
| `mysql.host` | 确认非敏感 MySQL 主机 |
| `mysql.port` | 确认 MySQL 端口 |
| `mysql.user` | 确认 MySQL 用户名 |
| `mysql.database` | 确认目标数据库 |
| `smb.backend-repo` | SMB 后端仓库位置 |
| `smb.frontend-repo` | SMB 前端仓库位置 |
| `smb.idl-repo` | SMB IDL 仓库位置 |

`mysql.password` 不属于可查询键；请求它或任意未知键返回参数错误，且不泄露是否配置了密码。支持键尚未设置时返回运行错误，提示用户在项目或 Home 配置中设置该键。该命令不提供 `show`，避免未来新增敏感配置后被整体输出。

根帮助与 `easy config --help` 均须给出 `init`、`get`、配置位置和“密码不会通过 config get 输出”的简短说明。

## 7. Skill 与文档调整

### 7.1 SMB work order skill

`skills/smb-work-order/SKILL.md` 不再包含 `/Users/...` 形式的路径。它应要求 Agent 在需要定位 SMB 仓库时先执行：

```text
easy config get smb.backend-repo
easy config get smb.frontend-repo
easy config get smb.idl-repo
```

Agent 只能按本次任务实际需要读取相应仓库，不得因为三个配置均可获取就默认操作全部仓库。某个配置缺失时，Agent 必须请用户提供或设置路径，不能回退到历史硬编码位置。

### 7.2 聚合 skill 与 MySQL DDL skill

`easy-cli` 聚合 skill 的能力表增加配置初始化、查询入口，并说明 MySQL 命令会自动加载配置、显式 flag 可覆盖配置。`mysql-ddl-export` skill 的示例保留 `--password-stdin` 的安全建议，同时说明已配置的非敏感连接字段可以省略。

README 说明初始化命令、文件位置、覆盖规则、无密示例、MySQL 命令如何使用配置，以及 SMB 路径如何由配置提供。任何示例均不得包含真实密码或个人路径。

## 8. 内部结构

`internal/config` 是唯一的配置文件定位、严格 JSON 解码、字段级合并、安全键查询和 Home 初始化入口。初始化 API 负责模板内容、目录/文件权限、已有文件保护和原子替换；它不依赖 CLI 输出或 MySQL 驱动。

`cmd/easy` 使用当前工作目录和 Home 目录加载配置，并将结果或加载错误传入 `cli.Options`。`internal/cli` 仅在 MySQL 命令和 `config get` 中消费加载结果，使 skill 管理和帮助不受无关配置错误影响；`config init` 只接收 Home 路径且不依赖加载结果。MySQL 命令解析重构为“显式 flag 覆盖值 + 合并 + 校验”三个阶段。`internal/mysql` 继续只接收最终的 `ConnectionOptions`，不感知文件系统或配置优先级。

`internal/projectroot` 提供项目根定位，供 skill 安装和项目配置共同使用，避免两者对“当前项目”的判断产生差异。

## 9. 错误处理与安全

- 参数、未知配置键、重复 `--force` 或非法端口：退出码 `2`。
- 读取、解析、合并后缺少配置值、初始化目标已存在、初始化文件系统失败或数据库操作失败：退出码 `1`。
- 真实配置、示例配置、帮助文本、skill、README 和测试夹具中均不得出现真实密码。
- `config get` 只允许白名单非敏感字段，密码绝不写入 stdout、stderr、日志或错误。
- 不使用 shell 展开、`eval` 或命令拼接读取配置值；路径只作为 Go 文件系统路径处理。

## 10. 测试与验收

测试采用 TDD，且不依赖真实 MySQL 或真实 Home 目录：

1. `internal/config` 覆盖无配置、仅 Home、仅项目、字段级覆盖、项目根发现、非法 JSON、未知字段与字段类型错误。
2. CLI 覆盖 MySQL 配置补全、CLI 覆盖项目/Home、项目覆盖 Home、端口默认值、配置密码、stdin 密码覆盖配置密码以及合并后缺失字段。
3. `easy config get` 覆盖所有允许键、未设置键、未知键与 `mysql.password` 拒绝；断言输出与错误均不含配置密码。
4. SMB skill 测试/检查确认不存在个人绝对路径，且包含三个 `easy config get smb.*-repo` 指引。
5. `easy config init` 覆盖首次创建、模板合法性、目录/文件权限、已有文件不覆盖、`--force` 原子覆盖和非法参数。
6. 运行 `make check`、`make test-race` 与 `make build`；人工检查 `easy --help`、`easy config --help` 和 MySQL help 的描述。

验收时，用户可运行 `easy config init` 获得仅自己可读的 Home 配置模板，填写后可仅在 Home 配置保存公共默认值、在项目配置填写项目专属数据库或 SMB 路径，并通过一次命令验证项目字段覆盖 Home 字段；带显式 MySQL flag 的命令必须最终使用 flag 值。任何路径或密码缺失时，CLI 必须给出可操作错误，而不是猜测或使用旧的硬编码值。
