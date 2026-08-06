# 项目开发约定

## Spec 文件

- 所有设计 spec 必须直接放在 `docs/specs/` 下。
- spec 文件命名格式为 `YYYY-MM-DD-<topic>-design.md`。
- 禁止创建 `docs/<skill-name>/specs/`、`docs/superpowers/specs/` 等技能名子目录。
- spec 需要声明它涉及的模块，并在正文中保留清晰的“关联模块”信息。

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
