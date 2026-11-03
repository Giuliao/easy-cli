# 基于 Project / Task 的跨会话工作续接设计

## 背景与目标

新会话主要在原需求上下文上修问题、补功能；换会话或 agent 不应改变 project、仓库 task、worktree 或 commit 起点。Task 代表需求在某个仓库/开发目录中的持续开发记录，不是会话或每轮反馈的工作段。

保留 Project + Task 模型，仅增强当前绑定和 Git 进度记录。原 PR 中独立 Workspace、归档和按工作段新建 task 的设计撤回。本设计覆盖旧 Project/Task spec 的会话身份与收尾规则。

## 非目标

- 不引入 Workspace 模型、workspaces 表、归档或新的现场生命周期。
- 不因换会话、反馈修正或补功能自动新建 task；不提供 `work ensure --new-task`。
- 不创建/切换/删除 Git 分支或 worktree，不自动提交、rebase 或 reset。
- 不集成 agent hooks，不承诺每轮自动调用；不存完整对话、不推测需求验收结束。
- 不按 session、最近活跃或名称相似度猜测项目；不承诺 commit 范围是 agent 独占贡献。
- 不修改 `AGENTS.md`。

## 关联模块

- `internal/project`：worktree 本地 context、持续 task 恢复、SQLite 与 checkpoint。
- `internal/cli`：project/task/work 命令及输出。
- `skills/project-workflow`：原需求原 task 续接、需求文档阅读和明确关闭规则。
- `skills/easy-cli`：能力路由。
- `README.md`：用法与边界。

## 当前绑定

当前 worktree 根目录的 `.easy-cli/context` 写入 JSON：

```json
{"project_id":"...","project_name":"sales-order","task_id":"tsk_..."}
```

task_id 可空。文件仍兼容旧版纯 project name；ensure 首次恢复后升级 JSON。数据库 project_id 为稳定身份，项目删除后不能因重建同名而自动接上。读取失败、无效 JSON、未知 project 或失效 task 必须明确报错，不降级到猜测。各 linked worktree context 相互独立；子目录/symlink 解析到所属 Git root。

context 只存 project/task 指针，不复制 branch/directory/Git common dir，不创建单独的 Workspace 实体。project use 相同 project 保留原 task；跨 project 必须显式 --replace，清空 task 指针，不改变旧 task 或历史。删除/解除关联后的失效 task 指针不会自动继承其他 task，需明确 --task 选择。

## 命令设计

- `easy project use <name> [--dir <path>] [--replace]`：核对 active project，更新当前 worktree context，不创建 task、不切分支。相同 project 幂等并保留 task；冲突需明确替换。
- `easy project show <name> [--json]`：保留原 project + tasks 输出。各 task 的 directory/branch/commit_range 就是跨仓库现场清单，不新增 workspaces 字段。
- `easy work status [--dir <path>] [--json]`：只读当前 context、project、可选 task 和实际 Git 状态。展示失效指针、分支/目录/历史/lifecycle 不一致；不补写数据。
- `easy work ensure [--project <name>] [--dir <path>] [--agent <type>] [--task <id>] [--json]`：已有 context.task_id 就沿用；explicit --task 用于选择已存在的正确 task。首次无 task 时按 project + Git worktree root 匹配：唯一且 branch 一致则复用；多个需 --task；目录有旧 task 但 branch 不符报漂移，不新建；确实没有该目录的 task 才创建。新仓库才有新的持续开发记录。--agent 仅用于首次创建。反馈修正、补功能、换会话/agent 均不分段、不重置 base。
- `easy work checkpoint [--dir <path>] [--json]`：仅对当前明确绑定 task 采集 HEAD/dirty，固定 base、推进 head、追加 task checkpoint 历史；不调用 ensure/create，不改变 Git。
- `easy work history [--dir <path> | --task <id>] [--json]`：默认读当前 context.task_id；--task 可在通用目录查询 task 的全部历史，无需现场在线。没有归档/workspace 查询。
- `easy task create` / update：保留显式记录管理；--session 可选兼容溯源，不参与恢复。只有新仓库或用户明确要新增独立记录才创建 task；随后可 ensure --task 选定，不自动切分支。
- `easy project close <name> --confirm` / reopen：仅有用户明确指令后执行；本轮实现、总结、提交、会话结束均不关闭项目。confirm 只是误操作保护，不是授权证明。

## 持久化和一致性

原 projects/tasks/project_tasks 及 session 字段兼容保留。work_task_progress 只保存 task 的固定 base/head，用于防止手工改 commit_range 后悄悄重置范围，不承载现场模型。task_checkpoints 保存 id、task_id、branch、base_commit、head_commit、dirty、created_at，并以 task_id 外键关联；删除 task 清理其历史，删除 project 保留 task 和历史。

写入启用 immediate 事务、外键和 busy timeout。ensure 在数据库事务中重新读取 context、选择/创建 task、固定进度；最终以同目录临时文件 + rename 原子发布 context，再提交数据库。同一数据库并发 ensure 被序列化，不重复建 task。发布失败回滚数据库；commit 失败尽力恢复原 context，若恢复也失败给出明确错误。跨文件/SQLite 的进程崩溃无法提供绝对原子性，失效 task 指针 fail closed，不静默重建。checkpoint 与 task range/progress/history 在一个数据库事务更新；元信息 update 同样串行化，避免覆盖并发 checkpoint。

对已运行 PR 早期版本的本地数据库，仅做一次兼容清理：把原 work_checkpoints 中仍存在的 task 历史转入 task_checkpoints，再移除旧表和 workspace 相关触发器。保留全部 projects/tasks/progress；不将旧 Workspace 绑定写到任意目录，不修改历史 commit。原纯名称 context 通过正常唯一 task 恢复升级；多 task 时需显式选择。

## Git 与需求上下文边界

- 恢复前读取技术文档、最新需求边界和待解决反馈；Git/context 解决身份与提交范围，不能代替业务上下文。
- 初次 task 从 HEAD 固定 base..head（初始相等）；旧可解析 hash 范围保留起点并转完整 hash。旧空范围从当前 HEAD 开始，不能补采之前未记录历史。
- base 必须为旧 head 祖先、旧 head 必须为当前 HEAD 祖先；与 saved progress 不一致时拒绝覆盖。ensure 不推进 head，checkpoint 才推进。
- task branch/所属 Git root/项目关联不一致、reset/rebase、非法 hash、已关闭项目拒绝静默续接；人工核对，不靠新 task 绕过错误。
- Git dirty 包含暂存、未暂存、非忽略 untracked；context 自身 bookkeeping 排除。无需提交才能记录。
- 普通 linked worktree、subdir、symlink 支持；已有 commit 与非 detached 分支必需。linked worktree 元数据移出标准 .git 后若无法识别主仓库则明确报错，主 worktree separate/symlink 元数据仍支持。

## 测试与验收标准

1. 新数据库不建立 workspaces，旧 Project/Task 和早期 PR checkpoint 可迁移且不丢失。
2. 多次 ensure、重启 CLI、换 agent、修问题/补功能都复用同 task，base 不变，无 --new-task。
3. 旧 context、JSON context、多个候选、失效 task、project ID/name 冲突正确处理，不猜测。
4. linked worktree context 隔离，子目录/symlink task 恢复；branch 漂移不新建记录。
5. checkpoint 固定 base、推进 head并记录 dirty，history --task 可离线查询，元信息并发更新不丢进度。
6. 项目关闭需要明确确认，project show 无 workspaces，AGENTS.md 与 main 一致。
7. 单元、真实 Git/context/数据库迁移和 CLI 测试通过；go test ./...、race、vet、build 通过。
