# spine233-agent-cli

Go 1.26 实现的本地 Spine Pro Agent CLI。接入
[`spine233-file-parser`](https://github.com/neko233-com/spine233-file-parser)，
同时提供机器可读 CLI 与 stdio MCP，支持 Codex、Claude Code、Cursor 等 Agent。

## 能力

- 检测 `.spine`、`.skel`、Spine JSON。
- 无需启动编辑器，检查 `.spine` 私有容器元数据与诊断文件。
- 汇总骨骼、插槽、皮肤、事件、动画。
- 用 RFC 6901 JSON Pointer 精确读取语义数据。
- 预览或写出 JSON Patch；只支持 `add`、`replace`、`remove`。
- 通过已安装、已授权的 Spine Pro CLI 执行 `.spine ↔ JSON`。
- 7 个 MCP 工具；本地 stdio，无网络、账号、遥测。

Spine 项目语义格式私有且随版本变化。完整项目导入/导出统一通过官方
Spine Pro CLI；本项目不绕过 Spine 授权。

## 安装

```bash
go install github.com/neko233-com/spine233-agent-cli/cmd/spine233-agent-cli@latest
```

需要语义导入/导出时配置已授权 Spine：

```powershell
$env:SPINE_EXECUTABLE = "D:\IDE\Spine\Spine.com"
```

```bash
export SPINE_EXECUTABLE=/opt/Spine/Spine
```

也可为 `export`、`import` 单独传 `--spine`。

## CLI

所有结果写 stdout JSON；错误写 stderr JSON。

```bash
spine233-agent-cli detect --file hero.spine
spine233-agent-cli inspect --file hero.spine --output-dir .spine-diagnostics
spine233-agent-cli summarize --file hero.json
spine233-agent-cli query --file hero.json --pointer /animations/walk

spine233-agent-cli export \
  --file hero.spine \
  --output-dir exported \
  --editor-version 4.3.xx

spine233-agent-cli import \
  --json exported/hero.json \
  --output restored.spine \
  --skeleton-name hero
```

JSON 修改默认仅预览：

```bash
spine233-agent-cli patch \
  --file hero.json \
  --operations '[{"op":"replace","path":"/bones/1/name","value":"arm-new"}]'
```

确认后写入新文件。输入文件永不被覆盖：

```bash
spine233-agent-cli patch \
  --file hero.json \
  --output hero-edited.json \
  --operations '[{"op":"replace","path":"/bones/1/name","value":"arm-new"}]' \
  --apply
```

## Agent / MCP 接入

服务命令：

```bash
spine233-agent-cli serve
```

Codex 配置示例：

```toml
[mcp_servers.spine233]
command = "spine233-agent-cli"
args = ["serve"]
env = { SPINE_EXECUTABLE = "D:\\IDE\\Spine\\Spine.com" }
```

Claude Desktop 配置示例：

```json
{
  "mcpServers": {
    "spine233": {
      "command": "spine233-agent-cli",
      "args": ["serve"],
      "env": {
        "SPINE_EXECUTABLE": "D:\\IDE\\Spine\\Spine.com"
      }
    }
  }
}
```

MCP 工具：

| 工具 | 用途 |
| --- | --- |
| `spine_detect` | 检测文件类型 |
| `spine_summarize` | 紧凑汇总项目或骨架 |
| `spine_inspect_project` | 检查 `.spine` 并保存诊断 |
| `spine_export_project` | Spine Pro 导出 `.spine → JSON` |
| `spine_import_project` | Spine Pro 导入 `JSON → .spine` |
| `spine_query_json` | JSON Pointer 精确读取 |
| `spine_patch_json` | 预览或写出语义修改 |

JSON Pointer 返回默认限制为 1 MiB，可用 `maxBytes`/`--max-bytes` 调整至最多
16 MiB；超过限制时应改用更窄的 Pointer。汇总中每类名称最多返回 500 个，
完整数量保留在 `counts`。

`spine_patch_json` 默认 `apply=false`。落盘必须设置 `apply=true` 和不同于输入的
`outputPath`。导入默认拒绝覆盖已有 `.spine`，需显式 `overwrite=true`。

## 开发

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o bin/spine233-agent-cli ./cmd/spine233-agent-cli
```

## License

MIT。Spine 是 Esoteric Software LLC 的商标。Spine Editor 与 Spine Runtimes
使用其各自许可证。
