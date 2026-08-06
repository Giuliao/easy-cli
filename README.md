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

## 开发

```bash
go test ./...
go build ./cmd/easy
```

内置 skill 位于 `skills/<name>/SKILL.md`，通过 Go `embed` 打包进二进制。新增 skill 后重新构建即可出现在 `easy skill list` 和同名快捷命令中。
