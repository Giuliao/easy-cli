# easy

基于 Go 的 skill/prompt CLI 工具：把开发约束维护在代码仓库中，经过本地结构压缩后输出，或安装为 Codex 可识别的 skill。

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

## 开发

```bash
go test ./...
go build ./cmd/easy
```

内置 skill 位于 `skills/<name>/SKILL.md`，通过 Go `embed` 打包进二进制。新增 skill 后重新构建即可出现在 `easy skill list` 和同名快捷命令中。
