# easy

基于 Go 的 skill/prompt CLI 工具：把开发约束维护在代码仓库中，经过本地结构压缩后输出，或安装为 Codex 可识别的 skill。

## 安装

一键安装（macOS / Linux）：

```bash
curl -fsSL https://raw.githubusercontent.com/Giuliao/easy-cli/main/install.sh | sh
```

脚本会自动识别操作系统和架构，从 GitHub Releases 下载对应二进制并校验 SHA-256。默认安装到 `~/.local/bin/easy`（root 用户安装到 `/usr/local/bin/easy`）。

可通过环境变量自定义：

```bash
# 安装到指定目录
EASY_INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/Giuliao/easy-cli/main/install.sh | sh

# 安装指定版本
EASY_VERSION=v0.2.0 curl -fsSL https://raw.githubusercontent.com/Giuliao/easy-cli/main/install.sh | sh
```

也可以直接从 [Releases](https://github.com/Giuliao/easy-cli/releases) 下载对应平台的二进制。

## 构建

```bash
go build -o easy ./cmd/easy
```

## 使用

```bash
./easy skill list
./easy skill show smb-work-order
./easy skill prompt smb-work-order
./easy smb-work-order
```

`prompt` 默认输出压缩后的 Markdown；`--raw` 输出原始 skill，`--format json` 输出原始内容和压缩内容：

```bash
./easy skill prompt smb-work-order --raw
./easy skill prompt smb-work-order --format json
```

## 配置

easy 会先读取 Home 配置 `~/.config/easy-cli/config.json`，再读取当前 Git 项目的 `.easy-cli/config.json`；项目文件按字段覆盖 Home 文件。初始化仅当前用户可读的 Home 模板：

```bash
./easy config init
```

目标已存在时命令不会覆盖。只有明确要重置 Home 配置时才使用 `./easy config init --force`。初始化不会修改项目配置。真实项目配置已被 Git 忽略；可从 [.easy-cli/config.example.json](.easy-cli/config.example.json) 创建：

```bash
mkdir -p .easy-cli
cp .easy-cli/config.example.json .easy-cli/config.json
```

配置可保存 MySQL 的 `host`、`port`、`user`、`password`、`database`，以及 SMB 后端、前端和 IDL 仓库路径。优先级为：

```text
命令行显式参数 > 项目配置 > Home 配置 > MySQL 端口默认值 3306
```

查看合并后的非敏感值：

```bash
./easy config get smb.backend-repo
./easy config get mysql.database
```

`easy config get` 永远不会输出 `mysql.password`。若连接字段都已经配置，MySQL 命令可省略这些 flag：

```bash
./easy mysql ddl
./easy mysql query --sql 'SELECT id, name FROM users LIMIT 20'
```

SMB skill 会通过 `easy config get smb.*-repo` 获取仓库位置；未配置时会要求提供路径，而不会使用写死的个人目录。

## 外置 skill

easy 会自动发现两级本地 skill 目录：

```text
~/.config/easy-cli/skills/<name>/SKILL.md
<project-root>/.easy-cli/skills/<name>/SKILL.md
```

项目级 skill 覆盖 Home 级同名 skill。发现后可直接使用：

```bash
./easy skill list
./easy skill show <name>
./easy skill prompt <name>
./easy <name>
```

外置 skill 不会自动安装到 `.agents/skills`。只有显式执行 `easy skill install <name>` 才会安装。更新聚合入口时：

```bash
./easy skill install easy-cli
./easy skill update easy-cli
```

`easy-cli` 只写入外置 skill 的名称、描述和按需加载命令，不复制外置 skill 正文；新增或修改外置 skill 后重新执行 `update easy-cli` 即可刷新已安装的聚合入口。

## 通过 easy-cli 使用其他 skill

安装 `easy-cli` 聚合 skill 后，Agent 应把它作为能力路由和命令行用法说明：先选择并调用对应 skill，不要因为识别到 skill 就自动安装。只有用户明确要求管理 skill 时，才执行安装或更新命令。

```bash
# 发现可用能力
./easy skill list

# 读取其他 skill 的压缩 prompt
./easy skill prompt smb-work-order
./easy smb-work-order
./easy skill prompt mysql-ddl-export

# 直接执行内置功能
./easy mysql ddl
```

## 安装 skill

默认安装到当前项目的 `.agents/skills/`：

```bash
./easy skill install smb-work-order
```

安装到用户级 `~/.agents/skills/`：

```bash
./easy skill install smb-work-order --global
```

目标文件内容不同的时候，安装器默认拒绝覆盖；确认后使用 `--force`：

```bash
./easy skill install smb-work-order --force
```

安装和更新大一统 `easy-cli` skill：

```bash
./easy skill install easy-cli
./easy skill install easy-cli --global
./easy skill update easy-cli
./easy skill update easy-cli --global
```

`update` 只更新已经安装的 skill；未安装时请先执行 `install`。安装 `easy-cli` 只安装聚合 skill 本身，不会自动安装其他子 skill。

## 导出 MySQL DDL

只导出目标数据库中的普通表定义，DDL 按表名排序后输出到 stdout：

```bash
./easy mysql ddl \
  --host 127.0.0.1 \
  --port 3306 \
  --user root \
  --password 'your-password' \
  --database app
```

也可以从 stdin 读取密码，避免密码进入 shell 历史：

```bash
printf '%s\n' 'your-password' | ./easy mysql ddl \
  --host 127.0.0.1 \
  --user root \
  --password-stdin \
  --database app
```

第一版不导出视图、触发器、存储过程、函数或数据。

## 查询 MySQL 数据

使用 `mysql query` 执行用户提供的 SQL，默认输出便于 Agent 处理的 JSON：

```bash
printf '%s\n' "$MYSQL_PASSWORD" | ./easy mysql query \
  --host 127.0.0.1 \
  --user root \
  --password-stdin \
  --database app \
  --sql 'SELECT id, name FROM users LIMIT 20'
```

也可以使用制表符格式查看结果：

```bash
printf '%s\n' "$MYSQL_PASSWORD" | ./easy mysql query \
  --host 127.0.0.1 \
  --user root \
  --password-stdin \
  --database app \
  --sql 'SELECT id, name FROM users LIMIT 20' \
  --format table
```

该命令不会限制 SQL 语句类型，会将 SQL 原样提交给 MySQL；执行前请确认目标数据库和语句可能产生的写入影响。

## Project / Worktree 跨会话续接

工作身份属于 project 与实际 Git worktree；task 是该仓库的持续开发记录，跨会话、跨 agent 复用，不是一次 agent 会话。新会话或切换 agent 应先恢复既有现场，不自动新建分支、worktree 或 task。

```bash
# 在已知工作区中查看绑定和实际 Git 状态，再幂等恢复当前 task
./easy work status --dir /path/to/worktree --json
./easy work ensure --dir /path/to/worktree --agent codex --json

# 从通用目录启动时，先查看项目已有的 tasks
./easy project show <name> --json
```

没有绑定时，先确认项目和目录；只为确实的新项目执行 `project create`：

```bash
./easy project create <name> --description "目标"
./easy project use <name> --dir /path/to/worktree
./easy work ensure --project <name> --dir /path/to/worktree --agent codex
```

`project use` 只把当前 worktree 的 `.easy-cli/context` 指向项目，不创建 task；同一项目重复执行是幂等的，并保留 current task。项目冲突必须由用户决定，确认后才用 `--replace`：context 改指新项目并清空 task 指针，旧 project、task 和 checkpoint 历史原样保留，既不迁移也不归档，下一次 `work ensure` 为新项目建立它自己的 task。不要按会话 ID、最近活跃项目或名称相似度猜测现场。

`work ensure` 重复执行会返回同一个 current task；`--agent` 只用于首次创建。没有 task 指针时，按 project + worktree + branch 匹配已有 task：唯一则复用，多个必须用 `--task <id>` 明确选择，确实没有该目录的记录才创建。分支漂移、失效指针或指针指向别的项目一律报错，不会新建记录、重置 base 或继承其他 task。新会话、换 agent、修问题、补功能都不是新增工作段的理由；task 是该仓库的持续开发记录，随每次反馈继续推进：

```bash
# 多个候选时显式选择已核对的 task
./easy work ensure --project <name> --dir /path/to/worktree --task <id>

# 记录进展及交接，不自动创建 task 或提交 Git 变更
./easy work checkpoint --dir /path/to/worktree --json
./easy work history --dir /path/to/worktree --json

# 不需要现场在线时，按 task id 查询全部历史
./easy work history --task <id> --json
```

`checkpoint` 固定 task 的 base、向当前 HEAD 推进 head，记录暂存、未暂存和非忽略 untracked 变更的 dirty 标记，并追加历史。范围表示工作段，不保证是某个 agent 的独占贡献。分支/绑定不一致或 reset/rebase 造成历史分叉时，`ensure` / `checkpoint` 拒绝覆盖；先人工核对，不自动切分支、改范围或重建 task。

`easy task create` / `update` 仍用于显式管理记录；`--session` 仅为可选的旧数据溯源字段，不生成、不要求，也不用于续接。显式 `task create` 不会自行成为 worktree 的 current task，需要 `work ensure --task` 才会被选定。

一轮工作结束只做 checkpoint 和交接，不自动关闭项目。仅在用户明确要求关闭或重新打开项目后执行：

```bash
./easy project close <name> --confirm
./easy project reopen <name> --confirm
```

Git 边界：需要已有 commit 和非 detached 分支。普通 linked worktree、子目录和 symlink 路径可续接；少数 linked worktree 将共享 Git 元数据移出标准 `.git` 布局后，Git 无法可靠给出主工作区身份，此时命令明确拒绝并提示选择主工作区或恢复标准布局。

`--confirm` 只是误操作保护，不能代替用户授权。上述 status / ensure / checkpoint / history 是供 agent 或外部宿主调用的原语；尚未集成具体 agent hook，不保证每轮自动调用。完整 SOP 可通过 `./easy skill prompt project-workflow` 按需读取；设计与错误边界见 [跨会话续接设计](<docs/specs/2026-11-03-worktree-project-resume-design.md>)。

## 开发

```bash
go test ./...
go build ./cmd/easy
```

内置 skill 位于 `skills/<name>/SKILL.md`，通过 Go `embed` 打包进二进制。新增 skill 后重新构建即可出现在 `easy skill list` 和同名快捷命令中。
