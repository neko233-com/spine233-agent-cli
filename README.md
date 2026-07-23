# spine233-agent-cli

Go 1.26 本地 Spine Pro Agent CLI。接入
[`spine233-file-parser`](https://github.com/neko233-com/spine233-file-parser)，
提供机器可读 CLI 与 stdio MCP。

## 原则

- 独立二进制。
- 不启动、不调用、不依赖 `Spine.exe` / `Spine.com`。
- `.spine`：raw-DEFLATE 解包 → 临时 payload 修改 → 重新封包。
- 所有写操作 preview-first；永不覆盖输入。
- 无网络、账号、遥测。

## 能力

- 检测 `.spine`、`.skel`、Spine JSON。
- `.spine` 无损解包、检查、重新序列化。
- 直接定位动画记录，fail-closed 修改大端 float32 关键帧。
- Spine JSON 深度分析、引用验证、查询、Patch。
- Spine JSON 动画克隆、重定时、骨骼时间线替换。
- 9 个 stdio MCP 工具。

## 安装

```bash
go install github.com/neko233-com/spine233-agent-cli/cmd/spine233-agent-cli@latest
```

## CLI

stdout 仅输出 JSON；错误写 stderr JSON。

```bash
spine233-agent-cli detect --file hero.spine
spine233-agent-cli inspect --file hero.spine --output-dir .spine-diagnostics
spine233-agent-cli summarize --file hero.spine
```

直接 `.spine` 动画修改，默认只预览：

```bash
spine233-agent-cli animate-project --recipe demo/hero/agent-animation.json

spine233-agent-cli animate-project \
  --file hero-human.spine \
  --animation attack \
  --target-animation attack-agent \
  --end-before crouch \
  --edits '[{"from":13.22,"to":24,"expectedMatches":1}]'
```

确认后输出新工程：

```bash
spine233-agent-cli animate-project \
  --recipe demo/hero/agent-animation.json \
  --apply

spine233-agent-cli animate-project \
  --file hero-human.spine \
  --output hero-agent.spine \
  --animation attack \
  --target-animation attack-agent \
  --end-before crouch \
  --edits '[{"from":13.22,"to":24,"expectedMatches":1}]' \
  --apply
```

`expectedMatches` 不相等时操作失败，避免版本/布局漂移造成误改。

Spine JSON 操作：

```bash
spine233-agent-cli analyze --file hero.json
spine233-agent-cli validate --file hero.json
spine233-agent-cli query --file hero.json --pointer /animations/walk
spine233-agent-cli patch \
  --file hero.json \
  --operations '[{"op":"replace","path":"/bones/1/name","value":"arm-new"}]'
```

## Agent / MCP

```bash
spine233-agent-cli serve
```

Codex：

```toml
[mcp_servers.spine233]
command = "spine233-agent-cli"
args = ["serve"]
```

MCP 工具：

| 工具 | 用途 |
| --- | --- |
| `spine_detect` | 检测文件 |
| `spine_summarize` | 紧凑汇总 |
| `spine_inspect_project` | `.spine` 解包诊断 |
| `spine_patch_project_animation` | 直接修改 `.spine` 动画关键帧 |
| `spine_query_json` | JSON Pointer 查询 |
| `spine_patch_json` | JSON Patch |
| `spine_analyze_json` | 深度能力清单 |
| `spine_validate_json` | 引用验证 |
| `spine_clone_animation` | JSON 动画生成 |

## Demo

`demo/alien`、`demo/hero`、`demo/raptor` 各保留：

- `<角色>-human.spine`：官方人工 Pro 工程。
- `<角色>-agent.spine`：本 CLI 直接生成工程。
- `agent-animation.json`：可审计生成 recipe。
- 官方图片、导出资源、许可。

## 开发

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o bin/spine233-agent-cli ./cmd/spine233-agent-cli
```

## License

MIT。Spine 是 Esoteric Software LLC 商标。Demo 资产遵守各目录许可。
