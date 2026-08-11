# MySQL 数据查询 CLI 设计

## 1. 背景与目标

当前 easy CLI 支持通过 `easy mysql ddl` 导出数据库中普通表的 `CREATE TABLE` 定义。本次增加通用 SQL 数据查询能力，使用户和 Agent 可以通过命令行把任意 SQL 交给 MySQL 执行并读取结果。

目标命令：

    easy mysql query --host <host> --user <user> --database <database> --sql <statement>

查询使用与 DDL 相同的连接参数、密码输入方式和超时控制。SQL 不经过语句类型校验或只读白名单过滤，CLI 按用户输入原样提交给数据库；因此 `UPDATE`、`DELETE`、DDL 等语句也不会被 CLI 拦截，帮助信息和聚合 skill 必须明确提示该行为及其风险。

## 2. 非目标

- 不实现 SQL 解析、格式化、改写或权限判断。
- 不增加默认 LIMIT、分页或结果行数截断；结果范围由用户 SQL 决定。
- 不把查询结果写入文件或数据库。
- 不改变现有 `easy mysql ddl` 的普通表导出行为。
- 不新增独立的 MySQL 数据查询 skill；能力入口由 `easy-cli` 聚合 skill 维护。

## 3. 关联模块

- `cmd/easy`
- `internal/cli`
- `internal/mysql`
- `skills/easy-cli`

README 中的使用说明和 `docs/specs/index.md` 的双向索引也需要同步更新。

## 4. 命令设计

### 4.1 参数

```text
easy mysql query --host <host> --user <user> --database <database> --sql <statement> [options]

Options:
  --host <host>             MySQL host
  --port <port>             MySQL port (default: 3306)
  --user <user>             MySQL user
  --password <password>     MySQL password
  --password-stdin          Read password from stdin
  --database <database>     Database name
  --sql <statement>         SQL sent to MySQL without CLI filtering
  --format <format>         json or table (default: json)
  -h, --help                Show this help
```

`--password` 和 `--password-stdin` 互斥；密码优先通过 stdin 传入。`--sql` 必须存在且非空，缺失或空值返回参数错误。`--format` 只接受 `json` 和 `table`。

### 4.2 执行语义

1. 解析连接参数、SQL 和输出格式。
2. 使用现有 MySQL 连接构造和 30 秒 context timeout。
3. 将 SQL 原样传给 `database/sql`，不添加 LIMIT、不拼接额外条件、不检查语句类型。
4. 读取数据库返回的列信息和行数据，按数据库返回顺序保留列和行。
5. 将格式化后的结果写入 stdout；连接、执行和格式化错误写入 stderr。

查询命令不承诺 SQL 是只读的。帮助文本和 `easy-cli` skill 需要提醒用户：SQL 可能改变数据库内容，执行前必须确认目标连接、数据库和语句。

## 5. 输出格式

### 5.1 JSON（默认）

使用一个固定 envelope 保留列顺序、重复列名和行顺序：

```json
{
  "columns": ["id", "name"],
  "rows": [[1, "alice"], [2, "bob"]]
}
```

空结果仍输出合法 JSON：

```json
{"columns":["id","name"],"rows":[]}
```

这样不依赖列名唯一性，也方便 Agent 稳定解析动态 SQL 结果。

值转换规则：

- SQL NULL 输出为 JSON `null`。
- 数字、布尔值保留 JSON 原生类型。
- `time.Time` 使用 RFC3339Nano 字符串。
- UTF-8 字节值输出为字符串；无法按 UTF-8 表示的字节值使用可识别的 base64 字符串表示，避免静默丢失数据。

### 5.2 table

`table` 输出制表符分隔的列头和行，列、行顺序与 JSON 一致。值使用与 JSON 相同的字符串化规则；空结果只输出列头（如果数据库返回列信息）。该格式主要用于终端人工查看和管道处理，不负责自动对齐列宽。

## 6. 内部结构

- `internal/mysql` 新增查询结果模型和基于 `*sql.DB` 的查询函数。
- `internal/mysql` 负责连接、执行、读取列/行和数据库值的稳定转换，不负责 CLI 参数解析。
- `internal/cli` 新增 `mysql query` 路由，解析 query 专属参数并调用查询函数。
- `internal/cli` 负责 JSON/table 格式化和帮助文本；通过依赖注入保留单元测试能力。
- `skills/easy-cli/SKILL.md` 增加 MySQL query 能力映射和“不限制 SQL、执行前确认风险”的说明。

现有 DDL 参数解析和导出函数保持兼容；可以提取共享的连接参数解析逻辑，但不得改变 DDL 的错误信息、密码保护和输出行为。

## 7. 错误与安全

- 缺失 `--host`、`--user`、`--database`、`--sql` 或密码参数返回退出码 2。
- 端口、格式和互斥密码参数错误返回退出码 2。
- 连接失败、SQL 执行失败或读取结果失败返回退出码 1。
- 不在 stdout/stderr 输出密码、完整 DSN 或包含密码的连接字符串。
- 不替换用户 SQL 中的参数、不自动重试、不切换数据库或主机。
- SQL 的写入风险由用户确认，CLI 只负责准确执行并报告数据库返回的错误。

## 8. 测试与验收

### 8.1 数据库层

- 查询返回列名和多行数据，并保持顺序。
- 空结果返回空 rows 和合法列信息。
- NULL、数字、字符串、时间和字节值转换稳定。
- SQL 执行错误、读取错误和 context 超时正确返回。

### 8.2 CLI 层

- 解析完整连接参数、`--sql` 和默认 JSON 格式。
- `--format table` 输出列头和行。
- `--password-stdin` 能读取密码且不泄漏到输出。
- 不对 SQL 内容做 SELECT/只读限制，任意非空 SQL 都传给注入的查询函数。
- 缺参、非法格式和密码冲突返回正确退出码。
- `easy mysql query --help` 展示用途、参数和“SQL 不做 CLI 过滤”的说明。

### 8.3 集成验收

- `easy-cli` skill 能指导 Agent 使用 `easy mysql query`，不会把数据查询误导为 skill 安装。
- README、根帮助、MySQL query 帮助和 spec 索引同步更新。
- `make check`、`make test-race`、skill quick validation 和 `make build` 全部通过。
