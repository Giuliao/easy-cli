# 基于 Project / Worktree 的跨会话工作续接设计

## 背景与目标

需求验收可能跨越多个 AI 会话；会话结束、上下文压缩或切换 agent 不改变需求和开发现场。按用户已确认方案，工作身份属于 project 与 worktree，task 代表一段可跨会话接力的工作，而非会话本身。

## 非目标

- 不创建、切换、删除 Git 分支或 worktree，不自动提交或改写历史。
- 不集成特定 agent 的 hook，不声称 CLI 能保证每轮自动调用。
- 不按会话 ID、最近活跃项目或名称相似度自动选择工作。
- 不新增需求验收状态机、完整对话存档或独占贡献统计。

## 关联模块

- `internal/project`：SQLite 向后兼容迁移、工作区绑定、幂等续接、Git checkpoint。
- `internal/cli`：project/work/task 命令、JSON 与文本输出。
- `skills/project-workflow`：恢复优先、跨会话 task、用户确认关闭的开发规则。
- `skills/easy-cli`：能力路由。
- `README.md`：跨会话用法与限制。

本设计覆盖旧 Project/Task spec 中的会话身份、上下文和收尾规则；旧 CRUD 与数据保留。

## 命令设计

- `easy project use <name>`：检查 project 为 active，绑定当前真实 Git worktree（规范化绝对路径、Git common dir、仓库名、分支）。保留 `.easy-cli/context` 名称格式作兼容。不同项目绑定冲突时必须显式 `--replace`；替换不能静默继承原 task。支持 `--dir` 选择工作区。不创建 task。
- `easy work status [--dir <path>] [--json]`：只读展示绑定的 project、workspace、可选 current task 与实际 Git 状态；列出分支/历史/生命周期不一致，不写记录。未绑定返回可操作错误。
- `easy work ensure [--project <name>] [--dir <path>] [--agent <type>] [--task <id> | --new-task] [--json]`：按 worktree 恢复。新会话、换 agent 不创建新 task；agent 只用于首次创建。已有 current task 直接返回。缺少 current task 时，复用唯一的同 project/目录/分支旧 task；多个候选必须用 `--task` 选择或显式 `--new-task`，不猜测。无绑定时可由明确 project 或旧 context 引导首次绑定。绑定冲突、非 active、分支漂移、历史不一致时拒绝。无候选才创建空 session 的 task，并固定初始 HEAD 为 base。`--new-task` 明确划分新的工作段，在原工作区创建 task，不新建分支。
- `easy work checkpoint [--dir <path>] [--json]`：仅对已有 current task 采集 Git，固定 base，推进 head，并记录 dirty 标记及追加不可变历史。不得自行 ensure/create；拒绝分支/common dir/关联不一致、非法范围、reset/rebase 导致旧 head 不再为祖先。不修改 Git、不自动提交。
- `easy work history [--dir <path> | --workspace <id>] [--json]`：按当前工作区展示 checkpoint 历史（包含旧 task）；显式 workspace ID 可读取归档或离线现场的历史，不混入其他项目。
- `easy project show <name>`：在现有 tasks 之外展示已绑定 workspaces，供从通用目录启动的新会话找到各仓库现场；不验证其他工作区是否在线。
- `easy project close <name> --confirm` 与 `easy project reopen <name> --confirm`：显式生命周期动作。workflow 只允许用户明确要求后执行；参数是误操作保护，不构成用户授权验证。
- `easy task create` 的 `--session` 改为可选，旧字段和 update 能力保留作溯源，不参与工作恢复。显式 task create 仍为新增工作记录，不自行设置 current task。

## 数据流与持久化

全局 SQLite 新增 `workspaces`：稳定 id、project_id、规范化 worktree directory（active 绑定唯一）、active、task_selection_required、git_common_dir、repository、branch、current_task_id（可空）、时间戳。跨 project/branch/common dir 的显式替换归档旧 workspace 与历史并建立新身份；不把 checkpoints 转移到新项目。同绑定 use 幂等，显式 replace 清空 current task。替换、删除或解除关联后要求显式 --task 或 --new-task，避免静默继承旧 task。分支和路径不是用于猜测项目的提示，而是绑定的校验事实。多个 linked worktree 各有独立目录，common dir 用于识别共享代码仓。删除 project 清理工作区和 checkpoints，保留原 tasks；删除 current task 清空指针，不继承其他 task。

新增 `work_task_progress` 独立固定 task 的 base/head（ensure 初次采集即保存），跨 workspace 重新选择旧 task 时也校验，不能通过手工更新 commit_range 重置 base。checkpoint 仅推进已保存 head。

新增 `work_checkpoints`：递增 id、workspace_id、task_id、branch、base_commit、head_commit、dirty、created_at。checkpoint 与 task range 更新在同一个事务内；ensure 创建 task、关联 project、保存 current task 在同一事务内。数据库连接启用外键、busy timeout，写事务使用 immediate 模式避免并发创建重复 task。

`.easy-cli/context` 只在没有数据库绑定时作为旧名称提示。数据库绑定优先，读错、名称冲突和删除的 project 不静默降级。新绑定不靠共享 common dir context，保持 worktree 隔离。路径通过 Git 根目录、绝对路径和 symlink 规范化；非 Git 目录、空仓库和 detached HEAD 明确报错。旧 task 的可解析目录也按所属 Git worktree 根目录规范化后匹配。普通 linked worktree 支持隔离；若 linked worktree 共享元数据移出标准 `.git` 布局且 Git 将元数据路径误报为主 worktree，无法可靠恢复主仓库身份，此时明确拒绝并提示选择主 worktree 或标准布局。主 worktree 的 separate/symlink 元数据布局仍支持。

## Commit 与错误处理

- 新 task 从创建时 HEAD 固定 `base..head`，初始两者相等；换会话不重置 base。
- 旧 task 的范围只允许两个可解析的十六进制 commit ID（可缩写），恢复时规范化为完整 hash；空范围从当前 HEAD 开始，文档提示无法补采未记录的历史。
- 采集前检查 base 为旧 head 的祖先，旧 head 为当前 HEAD 的祖先；有 checkpoint 时核对最新记录和 task range，防止手工改动悄悄覆盖。
- `work status` 报告现场漂移，ensure/checkpoint 拒绝覆盖；人工核对后可明确开始 `--new-task` 或使用另一个正确现场。分支漂移需要先恢复绑定分支，或明确重新绑定。
- dirty 覆盖暂存、未暂存与非忽略 untracked 文件。context 自身排除，不为记录而要求提交。
- 范围是可用于 `git log` 的工作段范围，不承诺是某个 agent 独占贡献；rebase 不自动追踪映射。

## 测试与验收标准

1. 旧 SQLite schema 可升级；旧 task/session 字段可读，session 不再必填。
2. 同 worktree 多次 ensure、重新启动 CLI 或切换 agent 都复用同 project/task，base 不变。
3. linked worktrees、子目录与 symlink 路径正确识别；无绑定、多个候选、显式 project 冲突均不猜测。
4. legacy context 和唯一旧 task 可恢复；多个旧 task 必须显式选择。
5. 显式 new-task 复用工作区，checkpoint 推进范围并存历史，未提交变更不会被提交。
6. 分支切换、reset、rebase、非法 range、task 关联改变、已关闭 project 拒绝静默续接。
7. close/reopen 无 confirm 不改变状态；本轮完成不关闭项目。
8. project show 提供跨仓库工作区清单，JSON 和文本路径可用。
9. 单元、真实临时 Git/worktree 集成、并发幂等与 CLI 重启测试通过；`go test ./...`、race、vet 和 build 通过。
