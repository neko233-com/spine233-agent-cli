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
- 自动解析 `.spine` 动画数量、名称、偏移和记录边界。
- preview-first 安全删除 `.spine` 动画表末项。
- 自动解析 `.spine` 骨骼名、对象偏移和原始父对象 token。
- 语义解析 rotate/translate/scale/shear、骨骼引用、帧、值和曲线。
- 解析并重定时 slot attachment 切换关键帧。
- 按骨骼引用、时间线、关键帧和通道 fail-closed 修改动画。
- 直接定位动画记录，fail-closed 修改大端 float32 关键帧。
- Spine JSON 深度分析、引用验证、查询、Patch。
- Spine JSON 动画克隆、重定时、骨骼时间线替换。
- 声明式重写整条 transform 时间线，适合 Codex 生成完整动作。
- 自动从已有动画生成 Codex 可编辑完整 recipe。
- 22 个 stdio MCP 工具。

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
spine233-agent-cli animations --file hero.spine
spine233-agent-cli bones --file hero.spine
spine233-agent-cli rotate-timelines --file hero.spine --animation attack
spine233-agent-cli transform-timelines --file hero.spine --animation attack
spine233-agent-cli slot-attachment-timelines --file alien.spine --animation death
spine233-agent-cli delete-last-project-animation \
  --file hero-human.spine \
  --animation walk
spine233-agent-cli scaffold-project-transform \
  --file hero-human.spine \
  --animation attack \
  --bone-references 6,12 \
  --timeline-types rotate,translate > attack-agent-recipe.json
```

删除动画仅允许当前目录末项，并校验预期名称；至少保留一个动画。默认预览，
确认后 `--apply` 写入同目录 `*-agent.spine`，永不覆盖输入：

```bash
spine233-agent-cli delete-last-project-animation \
  --file hero-human.spine \
  --animation walk \
  --apply
```

`--bone-references` 和 `--timeline-types` 可选；用于缩小大型工程的 agent
上下文。Codex 工作流：生成 recipe → 修改 `timelines[].keys` → 预览 → apply。

```bash
spine233-agent-cli rewrite-project-transform \
  --recipe attack-agent-recipe.json

spine233-agent-cli rewrite-project-transform \
  --recipe attack-agent-recipe.json \
  --apply

spine233-agent-cli compare-project-transform \
  --source hero-human.spine \
  --source-animation attack \
  --target hero-agent.spine \
  --target-animation attack-agent
```

紧凑批量程序可按骨骼、关键帧或帧范围执行 `set`、`add`、`multiply`。
`expectedMatches` 必填，防止工程漂移后误改：

```bash
spine233-agent-cli program-project-transform \
  --file hero-human.spine \
  --animation attack \
  --operations '[{"boneReferences":[6],"timeline":"rotate","channel":"value","keyIndices":[1,2],"mode":"add","operand":10,"expectedMatches":2}]'
```

语义修改 `.spine` rotate 动画，默认只预览：

```bash
spine233-agent-cli animate-project-transform --recipe demo/hero/agent-animation.json

spine233-agent-cli animate-project-transform \
  --file hero-human.spine \
  --animation attack \
  --target-animation attack-agent \
  --edits '[{"boneReference":6,"timeline":"rotate","keyIndex":1,"channel":"value","from":13.22,"to":24}]'
```

整条时间线重写：

```bash
spine233-agent-cli rewrite-project-transform \
  --file hero-human.spine \
  --output hero-agent.spine \
  --animation attack \
  --target-animation attack-agent \
  --timelines '[{"boneReference":6,"timeline":"translate","keys":[{"frame":0,"values":[-0.77,-1.89]},{"frame":5,"values":[8,-0.24]},{"frame":6,"values":[8.05,-2.44]},{"frame":12,"values":[-0.77,-1.89]}]}]' \
  --apply
```

重写保持时间线、关键帧数量不变；完整替换帧号和通道值，可选替换曲线。

确认后输出新工程：

```bash
spine233-agent-cli animate-project-transform \
  --recipe demo/hero/agent-animation.json \
  --apply

spine233-agent-cli animate-project-transform \
  --file hero-human.spine \
  --output hero-agent.spine \
  --animation attack \
  --target-animation attack-agent \
  --edits '[{"boneReference":6,"timeline":"rotate","keyIndex":1,"channel":"value","from":13.22,"to":24}]' \
  --apply
```

`boneReference`、`timeline`、`keyIndex`、`channel`、`from` 任一不符时失败，
避免布局或 Agent 计划漂移造成误改。rotate 专用和原始 float32 模式继续兼容。
`channel:"frame"` 可直接重定时；输出会校验每条时间线帧号严格递增。
`channel:"curve.x.0"` 等可修改已存储的通道曲线控制值。

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
| `spine_list_project_animations` | 直接列出 `.spine` 动画目录 |
| `spine_delete_last_project_animation` | 预览或删除动画表末项 |
| `spine_list_project_bones` | 直接列出 `.spine` 骨骼目录 |
| `spine_list_project_rotate_timelines` | 语义列出 rotate 时间线 |
| `spine_list_project_transform_timelines` | 列出骨骼变换时间线 |
| `spine_list_project_slot_attachment_timelines` | 列出 attachment 切换关键帧 |
| `spine_compare_project_transform_animation` | 验收 human/agent 动画名和语义差异 |
| `spine_build_project_transform_recipe` | 从已有动画生成完整或过滤后的 recipe |
| `spine_program_project_transform_animation` | 批量生成受 match 数保护的精确编辑 |
| `spine_patch_project_animation` | 直接修改 `.spine` 动画关键帧 |
| `spine_patch_project_rotate` | 语义修改 rotate 关键帧 |
| `spine_patch_project_transform` | 修改骨骼变换关键帧 |
| `spine_patch_project_slot_attachment` | 重定时已有 attachment 关键帧 |
| `spine_rewrite_project_transform_animation` | 声明式重写完整变换时间线 |
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
