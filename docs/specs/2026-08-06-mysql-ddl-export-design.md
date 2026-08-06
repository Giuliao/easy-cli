# MySQL DDL 导出 CLI 设计

## 1. 背景与目标

为 easy CLI 增加 MySQL schema DDL 导出能力。用户提供数据库连接信息，工具连接 MySQL 后读取普通表定义，并按稳定顺序将 `CREATE TABLE` DDL 输出到命令行 stdout。

第一版只处理普通表，不导出视图、触发器、存储过程、函数、事件或数据。

## 2. 命令接口

```bash
easy mysql ddl \
  --host 127.0.0.1 \
  --port 3306 \
  --user root \
  --password '***' \
  --database app
```

参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `--host` | 是 | MySQL 主机地址 |
| `--port` | 否 | 端口，默认 `3306` |
| `--user` | 是 | 登录用户名 |
| `--password` | 与 stdin 二选一 | 直接传入密码，便捷但可能进入 shell 历史 |
| `--password-stdin` | 与 password 二选一 | 从 stdin 读取一行密码，不回显 |
| `--database` | 是 | 目标数据库名 |
| `--help` | 否 | 显示命令帮助 |

`--password` 和 `--password-stdin` 不能同时使用。密码不写入 stdout、stderr、日志或错误包装信息。

## 3. 导出流程

1. 校验必填参数和互斥参数。
2. 使用 Go MySQL 驱动创建连接。
3. 通过 `database/sql` Ping 数据库，尽早报告认证、网络或权限错误。
4. 执行 `SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'` 获取普通表。
5. 按表名排序，避免 MySQL 返回顺序变化造成无意义 diff。
6. 对每张表执行 `SHOW CREATE TABLE`，读取完整建表定义。
7. 确保每条 DDL 以一个分号结束，表之间使用一个空行分隔。
8. 只将 DDL 写入 stdout；诊断错误写入 stderr。

无普通表时命令成功退出，stdout 为空。

表名作为 SQL 标识符使用反引号转义，反引号本身转义为两个反引号，避免表名造成 SQL 注入或查询错误。

## 4. 代码结构

```text
internal/mysql/
├── exporter.go
└── exporter_test.go

internal/cli/
├── cli.go
├── cli_test.go
└── mysql.go
```

`internal/mysql` 负责数据库连接配置、表发现和 DDL 导出，不依赖 CLI 输出；CLI 层负责参数解析、密码读取、stdout/stderr 和退出码。

数据库访问通过可测试的抽象隔离，单元测试不依赖真实 MySQL。真实运行时使用 Go MySQL 驱动。

## 5. 输出格式

示例：

```sql
CREATE TABLE `accounts` (
  `id` bigint NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE `orders` (
  `id` bigint NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

输出保留 MySQL `SHOW CREATE TABLE` 返回的完整定义，包括列、索引、约束、引擎、字符集、排序规则、注释和表选项。

## 6. 错误处理与安全

- 缺少参数或参数冲突：退出码 `2`，错误写入 stderr。
- 连接、认证、查询或读取失败：退出码 `1`，错误写入 stderr。
- 密码输入失败：退出码 `1`，不输出密码内容。
- 发现单张表导出失败：立即停止并返回错误，不输出不完整的成功提示。
- 不使用 shell 拼接 SQL 或连接命令。
- 不把完整 DSN 写入错误信息，避免密码泄漏。
- 使用 context 控制连接和查询生命周期。

## 7. 测试策略

采用 TDD：

- 表名列表按名称排序。
- 只处理 `BASE TABLE`，不包含视图。
- 每张表调用 `SHOW CREATE TABLE` 并保留完整 DDL。
- 表名中的反引号正确转义。
- 多张表输出顺序稳定，表间空行正确。
- 空数据库输出为空且成功。
- 数据库查询错误被返回并不会伪装成成功结果。
- CLI 参数缺失和 password/password-stdin 冲突返回退出码 `2`。
- `--password-stdin` 读取密码时 stdout 不包含密码。
- `easy mysql ddl` 端到端路由到导出功能。

验证命令：

```bash
make test
make vet
make build
```

## 8. 非目标与后续扩展

第一版不支持：

- 视图、触发器、存储过程、函数、事件。
- 数据导出。
- 直接写入文件或对象存储。
- 多数据库批量导出。
- SSH 隧道、TLS 自定义证书和云厂商专用认证。

后续可以增加 `--type`、`--output`、TLS 配置和对象过滤能力，但不改变默认的普通表 stdout 导出行为。
