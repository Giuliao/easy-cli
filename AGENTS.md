# 项目开发约定

## Spec 文件

- spec 数量未达到物理分目录阈值前，设计 spec 直接放在 `docs/specs/` 下。
- spec 文件命名格式为 `YYYY-MM-DD-<topic>-design.md`。
- 达到物理分目录阈值后，只允许在 `docs/specs/<domain>/` 下增加一层领域目录；禁止创建 `docs/<skill-name>/specs/`、`docs/superpowers/specs/` 或更深层级目录。
- spec 需要声明它涉及的模块，并在正文中保留清晰的“关联模块”信息。

### Spec 物理分目录阈值

- 当 `docs/specs/` 下的 spec 总数达到 15 个，或任一领域达到 5 个 spec 时，开始按领域物理分目录。
- 达到阈值时，在同一个变更中将现有 spec 统一迁移到一级领域目录，避免长期混合平铺和分目录两种布局。
- 领域目录使用小写 kebab-case，例如 `docs/specs/mysql/`、`docs/specs/easy-cli/`；目录最多一层。
- `docs/specs/index.md` 始终保留在根目录，并继续维护 Domain → Specs、Module → Specs、Spec → Modules 三类索引。
- 当前 spec 数量和 MySQL spec 数量都未达到阈值，因此当前阶段保持平铺，仅通过索引做逻辑归类。

## 模块与 Spec 双向索引

- `docs/specs/index.md` 是项目级双向索引，手工维护，不属于某个单独 spec。
- 每次新增或修改 spec 后，必须同步更新索引。
- 索引必须同时提供：模块 → spec，以及 spec → 模块。
- 模块使用仓库相对路径表示，例如 `internal/skill`、`internal/prompt`。
- 索引链接使用相对路径，保证仓库移动后仍然有效。
- `easy-cli` 不需要为该约定增加 CLI 命令、运行时代码或自动索引功能。

## Spec 工作流

- 先形成并确认设计，再进入实现计划和代码开发。
- spec 自检需要检查占位符、内部矛盾、范围和歧义。
- spec、`AGENTS.md` 和索引的变更应一起提交，避免约定与文档不同步。

## Spec 阅读与渐进式披露

- 阅读 spec 前先阅读 `docs/specs/index.md`，通过领域和模块索引筛选与当前任务相关的 spec。
- 第一轮只阅读目标 spec 的标题、背景与目标、非目标、关联模块、验收标准，先建立范围和约束。
- 进入设计或实现时，再按需阅读命令设计、数据流、内部结构、错误处理和测试章节；不需要为无关任务加载所有 spec 正文。
- 如果 spec 的章节之间存在约束关系，进入实现前必须补读相关章节，不能只读取摘要后自行推断。
