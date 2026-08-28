# Project 与 Task 管理 CLI 设计

## 背景与目标

`easy` CLI 目前提供 skill 管理和 MySQL 访问能力，但缺少对 AI agent 开发工作的追踪能力。
开发者在使用 codex、claude 等 agent 进行开发时，难以追踪：

- 当前有哪些进行中的项目
- 每个项目下有哪些 agent 会话在工作
- 每个 agent 会话在哪条分支、哪个目录、哪个 commit 区间内工作

本设计为 `easy` CLI 增加 project 和 task 管理能力，数据存储在全局 SQLite 数据库中，
同时支持人类可读的表格输出和 AI 友好的 JSON 输出。

## 非目标

- 不集成具体 agent 工具（codex、claude 等）的会话创建，会话 ID 由 agent 自行填入
- 不解析或验证 commit range 的 git 语义，仅作为字符串存储
- 不实现 task 级别的独立状态机，task 状态由所属 project 的状态决定
- 不实现多用户或权限控制，数据为单用户本地存储

## 关联模块

- `internal/project`：project 和 task 的数据模型、SQLite 存储、业务逻辑
- `internal/cli`：CLI 命令注册与输出格式化
- `internal/config`：复用配置目录定位逻辑
- `internal/projectroot`：复用 git 仓库根目录查找，用于上下文文件定位

## 数据模型

### SQLite Schema

数据库文件路径：`~/.config/easy-cli/easy.db`

```sql
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('active', 'ended')),
    ended_at    TEXT
);

CREATE TABLE tasks (
    id           TEXT PRIMARY KEY,
    agent_type   TEXT NOT NULL,
    session_id   TEXT NOT NULL,
    branch       TEXT NOT NULL,
    directory    TEXT NOT NULL,
    commit_range TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE project_tasks (
    project_id TEXT NOT NULL,
    task_id    TEXT NOT NULL UNIQUE,
    PRIMARY KEY (project_id, task_id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX idx_project_tasks_project_id ON project_tasks(project_id);
CREATE INDEX idx_project_tasks_task_id ON project_tasks(task_id);
```

### 字段说明

**projects 表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | UUID v4，主键 |
| name | TEXT | project 名称，全局唯一 |
| description | TEXT | project 描述，默认为空字符串 |
| created_at | TEXT | 创建时间，RFC3339 格式 |
| status | TEXT | 状态：`active`（进行中）或 `ended`（已结束） |
| ended_at | TEXT | 结束时间，RFC3339 格式，仅 `ended` 状态时有值 |

**tasks 表**

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT | 短 ID，格式 `tsk_YYYYMMDD_xxxxxx`（6 位十六进制随机数） |
| agent_type | TEXT | agent 类型，自由字符串（如 codex、claude） |
| session_id | TEXT | agent 会话 ID，由 agent 自行填入 |
| branch | TEXT | 开发分支 |
| directory | TEXT | 开发目录 |
| commit_range | TEXT | git commit range，如 `abc123..def456`，可选，默认为空字符串 |
| created_at | TEXT | 创建时间，RFC3339 格式 |
| updated_at | TEXT | 更新时间，RFC3339 格式 |

**project_tasks 表**

project 与 task 的关联表，实现一对多关系（一个 task 只能属于一个 project，
但通过中间表解耦，允许 task 先创建、后关联）。

### ID 生成规则

- **project id**：UUID v4
- **task id**：`tsk_` + UTC 日期（YYYYMMDD）+ `_` + 6 位十六进制随机数
  - 示例：`tsk_20260828_a1b2c3`

## 上下文机制

### 当前 Project 上下文

用户可通过 `easy project use <name>` 在当前 git 仓库下设置默认 project。
上下文存储在 `<projectroot>/.easy-cli/context` 文件中，内容为 project name。

当执行 `easy task create` 时：
1. 如果显式指定了 `--project <name>`，使用该 project
2. 否则，如果当前目录在 git 仓库内且存在 `.easy-cli/context` 文件，使用其中记录的 project
3. 否则，创建游离 task（不关联任何 project）

## 命令设计

### Project 命令

#### `easy project create <name> [--description <desc>]`

创建一个新的 project，状态为 `active`。

- 参数：`name`（必填，全局唯一）
- 选项：`--description`（可选，project 描述）
- 输出：创建成功的 project 信息

#### `easy project list [--json]`

列出所有 project，按创建时间倒序排列。

- 选项：`--json`（JSON 格式输出）
- 默认输出：无边框对齐表格

#### `easy project show <name> [--json]`

显示 project 详情，包括关联的 tasks。

- 参数：`name`（必填）
- 选项：`--json`
- 默认输出：project 信息 + 关联 tasks 表格

#### `easy project close <name>`

将 project 状态设为 `ended`，设置 `ended_at` 为当前时间。

- 参数：`name`（必填）

#### `easy project delete <name>`

删除 project 及其与 tasks 的关联关系（tasks 本身不删除）。

- 参数：`name`（必填）

#### `easy project use <name>`

在当前 git 仓库下设置默认 project，写入 `.easy-cli/context` 文件。

- 参数：`name`（必填）

### Task 命令

#### `easy task create --agent <type> --session <id> --branch <b> --dir <d> [--commit-range <r>] [--project <name>]`

创建一个新的 task。

必填选项：
- `--agent`：agent 类型
- `--session`：agent 会话 ID
- `--branch`：开发分支
- `--dir`：开发目录

可选选项：
- `--commit-range`：git commit range，AI 尚未提交时可省略
- `--project`：关联的 project name，不指定时按上下文机制处理

输出：创建成功的 task 信息

#### `easy task list [--project <name>] [--json]`

列出 tasks。

- 选项：
  - `--project <name>`：只列出该 project 下的 tasks
  - `--json`：JSON 格式输出
- 默认输出：无边框对齐表格

#### `easy task show <id> [--json]`

显示 task 详情。

- 参数：`id`（必填）
- 选项：`--json`

#### `easy task update <id> [--agent <type>] [--session <id>] [--branch <b>] [--dir <d>] [--commit-range <r>]`

更新 task 的字段，仅更新指定的字段，同时更新 `updated_at`。

- 参数：`id`（必填）
- 选项：任意可更新字段

#### `easy task delete <id>`

删除 task 及其与 project 的关联关系。

- 参数：`id`（必填）

#### `easy task attach <task_id> --project <name>`

将 task 关联到 project。

- 参数：`task_id`（必填）
- 选项：`--project <name>`（必填）

#### `easy task detach <task_id> --project <name>`

解除 task 与 project 的关联。

- 参数：`task_id`（必填）
- 选项：`--project <name>`（必填）

## 输出格式

### 表格输出（默认）

无边框对齐，类似 `kubectl get` 风格：

```
NAME        STATUS   CREATED                DESCRIPTION
my-project  active   2026-08-28T10:00:00Z  A sample project
```

列宽根据内容自适应，表头大写。

### JSON 输出（`--json`）

所有命令支持 `--json` 输出，返回结构化 JSON，便于 AI 解析。

示例（`easy project show my-project --json`）：

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "my-project",
  "description": "A sample project",
  "created_at": "2026-08-28T10:00:00Z",
  "status": "active",
  "ended_at": null,
  "tasks": [
    {
      "id": "tsk_20260828_a1b2c3",
      "agent_type": "claude",
      "session_id": "sess_123",
      "branch": "feature/foo",
      "directory": "/path/to/repo",
      "commit_range": "abc123..def456",
      "created_at": "2026-08-28T10:05:00Z",
      "updated_at": "2026-08-28T10:05:00Z"
    }
  ]
}
```

## 内部结构

### 包结构

```
internal/project/
  db.go          # SQLite 连接、初始化、迁移
  project.go     # Project 模型与 CRUD
  task.go        # Task 模型与 CRUD
  context.go     # 当前 project 上下文读写
  id.go          # ID 生成
```

### 关键类型

```go
type Project struct {
    ID          string
    Name        string
    Description string
    CreatedAt   time.Time
    Status      string // "active" | "ended"
    EndedAt     *time.Time
}

type Task struct {
    ID          string
    AgentType   string
    SessionID   string
    Branch      string
    Directory   string
    CommitRange string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 数据库初始化

- 使用 `modernc.org/sqlite`（纯 Go 实现，无需 CGO）
- 首次使用时自动创建数据库文件和表
- 启用 WAL 模式以支持并发读取
- 使用 `PRAGMA foreign_keys = ON` 启用外键约束

## 错误处理

| 场景 | Exit Code | 错误信息 |
|------|-----------|----------|
| 参数错误（缺少必填项、格式错误） | 2 | 明确指出哪个参数有问题 |
| project name 已存在 | 1 | `project "xxx" already exists` |
| project 不存在 | 1 | `project "xxx" not found` |
| task 不存在 | 1 | `task "xxx" not found` |
| task 已关联到该 project | 1 | `task "xxx" is already attached to project "yyy"` |
| task 未关联到该 project | 1 | `task "xxx" is not attached to project "yyy"` |
| 数据库错误 | 1 | 包含底层错误信息 |

## 测试

- `internal/project`：使用临时 SQLite 数据库测试 CRUD 操作
- `internal/cli`：测试命令行参数解析和输出格式
- 覆盖：project 创建/列表/查看/关闭/删除，task 创建/列表/查看/更新/删除/关联/解除关联

## 验收标准

1. `easy project create test-project` 创建成功，`easy project list` 能看到该 project
2. `easy project close test-project` 后，状态变为 `ended`，`ended_at` 有值
3. `easy task create --agent claude --session s1 --branch main --dir /tmp --commit-range abc..def --project test-project` 创建成功
4. `easy project show test-project` 能看到关联的 task
5. `easy task list --project test-project` 只列出该 project 下的 tasks
6. `easy task update <id> --commit-range new..range` 更新成功，`updated_at` 变化
7. `easy task detach <id> --project test-project` 后，task 不再关联该 project
8. `easy project use test-project` 在当前 git 仓库设置上下文后，`easy task create` 不带 `--project` 时自动关联
9. 所有命令支持 `--json` 输出，格式符合上述定义
10. 默认输出为无边框对齐表格
