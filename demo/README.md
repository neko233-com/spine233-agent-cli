# Official Spine validation projects

用于 `spine233-agent-cli` 与 `spine233-file-parser` 的本地回归验证。

来源：

- Repository: `https://github.com/EsotericSoftware/spine-runtimes`
- Branch: `4.3`
- Commit: `44db5210fc1dadd91065d5293e139e183ca64a44`

验证目录：

- `alien/`
- `hero/`
- `raptor/`

命名约定：

- `<角色>-human.spine`：官方 Pro、人工制作原版。
- `<角色>-agent.spine`：Agent 实际完成动画后的工程。

禁止把原版复制并重命名为 Agent 工程。`*-agent.spine` 只能由独立
`.spine` 语义解析与序列化链路生成并通过回归验证后提交。

Agent 动画：

| 角色 | 动画 | Agent 修改 |
| --- | --- | --- |
| alien | `death` → `death-agent` | 强化身体抽搐、反冲、倒地 |
| hero | `attack` → `attack-agent` | 强化蓄力、挥击、身体跟随 |
| raptor | `gun-grab` → `gun-grab-agent` | 强化前臂抓枪、伸展、回收 |

每个 `agent-animation.json` 保存 fail-closed 语义 recipe。操作直接解包
`.spine` raw-DEFLATE payload，按骨骼引用、rotate 关键帧索引及旧值三重校验，
修改后重新封包；不启动、不调用、不依赖 `Spine.exe` / `Spine.com`。

复现：

```bash
spine233-agent-cli animate-project-rotate --recipe demo/raptor/agent-animation.json
spine233-agent-cli animate-project-rotate --recipe demo/raptor/agent-animation.json --apply --overwrite
```

每个目录保持官方工程布局，包括 `.spine`、`images/`、官方导出文件和该
工程的 `license.txt`。这些文件仅用于测试与评估；使用者仍需遵守目录内许可
及 Spine Editor / Spine Runtimes 许可。

`validation/workspace/` 是下载、导出和诊断缓存，不提交。
