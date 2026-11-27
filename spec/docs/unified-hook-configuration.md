# Unified Hook Configuration

本文定义 SessionMan Protocol 的用户侧 Hook 配置。生命周期和能力边界由
[Unified Hook Lifecycle](unified-hook-lifecycle.md) 定义；所有实现同时遵守
[Unified Integration Principles](unified-integration-principles.md)。

## 设计原则

SessionMan 不发明新的 Hook DSL。用户配置采用 Claude Code 与 Codex 已使用的
`hooks.json` 结构；SessionMan 只定义其中能够跨 Claude、Codex、Gemini 和 Pi
运行的公共 profile。

配置语法与公共能力是两层：

- 语法层接受 Claude/Codex `hooks.json`。
- 公共能力层只保证五个公共事件和 command handler。
- 超出公共 profile 的合法原生配置产生 warning，不使整个配置失败。
- SessionMan 内部的 `session.start` 等名称不出现在用户配置中。

## 发现位置

Hook 定义只从 User Config 目录 `C` 读取：

```text
<C>/hooks.json
```

`C` 由 `--config-dir` 或 `SM_CONFIG_DIR` 指定。未指定 `C` 时，统一 Hooks 不生效。
不存在 user scope 或 project scope 的其他来源。Hook 之间不得依赖执行顺序。

文件内容兼容 Claude/Codex `hooks.json`，且 `<C>/hooks.json` 是统一层唯一的事实源。
原生 Provider 配置继续由原生 Provider 管理。

## 配置结构

```json
{
  "description": "Optional description",
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "./hooks/load-context.sh",
            "timeout": 10
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./hooks/check-prompt.sh",
            "timeout": 5
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./hooks/check-tool.sh",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./hooks/report.sh",
            "timeout": 10
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./hooks/cleanup.sh",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

### 公共事件

| 用户配置事件 | SessionMan Protocol 事件 |
|---|---|
| `SessionStart` | `session.start` |
| `SessionEnd` | `session.end` |
| `UserPromptSubmit` | `turn.before` |
| `PreToolUse` | `tool.before` |
| `Stop` | `turn.after` |

事件名区分大小写。

### Matcher group

每个事件包含若干 matcher group：

```json
{
  "matcher": "startup|resume",
  "hooks": []
}
```

- `hooks` 必须是非空数组。
- `matcher` 可省略；省略、空字符串和 `*` 都表示全部匹配。
- `SessionStart` 的 matcher 匹配公共 `source`。
- `SessionEnd` 的 matcher 匹配公共 `reason`。
- `UserPromptSubmit`、`PreToolUse` 和 `Stop` 不提供公共 matcher subject；其 `matcher` 被忽略并产生 warning。
- 为保证跨运行时一致，portable matcher 应只使用字面值、`|`、`^`、`$` 和 `.*`。
  更复杂但原生合法的正则可以保留，但不参与跨 Agent 投影。

### Command handler

公共 handler 为：

```json
{
  "type": "command",
  "command": "./hooks/check.sh",
  "timeout": 10
}
```

| 字段 | 要求 | 含义 |
|---|---|---|
| `type` | 必须为 `command` | 执行外部命令 |
| `command` | 必填非空字符串 | 与 Claude/Codex 相同的 shell command |
| `timeout` | 可选正数 | 秒；从进程启动到退出的总时限。`SessionEnd` 的 portable 值最大为 `3` |

命令以 session 的 `cwd` 作为工作目录。stdin 是一个 JSON 对象；stdout 只能是空输出、
纯文本或一个 JSON 对象。诊断日志必须写入 stderr。

同一事件的多个匹配 handler 可能并发执行。配置不得依赖声明顺序，也不得依赖另一个
handler 先完成。

`SessionEnd.timeout` 大于 `3` 是合法的原生配置，但超出 portable profile：SessionMan
产生 warning，且不跨 Agent 投影该 handler。这个限制来自公共目标中最短的可靠清理窗口；
`session.end` 本身仍然是 best-effort，不应承担强一致任务。

## 命令输入

公共输入保持 Claude/Codex 的 snake_case 形态。

所有事件：

```text
session_id
cwd
hook_event_name
transcript_path?
```

事件字段：

```text
SessionStart
  source                    # startup | resume | clear | fork | other
  parent_session_id?
  model?
  agent?

SessionEnd
  reason                    # completed | cleared | switched | interrupted | error | other
  last_assistant_message?
  started_at?
  ended_at?

UserPromptSubmit
  turn_id?
  prompt
  model?
  permission_mode?

PreToolUse
  turn_id?
  tool_call                 # { id, name, args }

Stop
  turn_id?
  outcome                   # completed | denied | interrupted | error
  last_assistant_message?
  error?
  duration_ms?
```

Provider 可以附加字段。portable Hook 必须忽略不认识的输入字段。

## 命令输出

用户侧输出继续采用 Claude/Codex 形态；SessionMan 在内部转换为统一决策。

### `SessionStart`

```json
{
  "systemMessage": "Workspace policy loaded.",
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "Read CONTRIBUTING.md before editing."
  }
}
```

`SessionStart` 不能阻止 session 创建。

### `SessionEnd`

```json
{
  "systemMessage": "Session audit saved."
}
```

输出不能阻止 session 结束，也不能修改 transcript。

### `UserPromptSubmit`

允许且不添加上下文时，退出 `0` 且不输出内容。允许并添加上下文时：

```json
{
  "systemMessage": "Repository policy applied.",
  "hookSpecificOutput": {
    "hookEventName": "UserPromptSubmit",
    "additionalContext": "Do not modify generated files."
  }
}
```

拒绝当前 Turn 时：

```json
{
  "decision": "block",
  "reason": "Confirmation is required before this operation."
}
```

退出码 `2` 且将原因写入 stderr 是等价的拒绝形式。其他非零退出码表示 Hook 失败，
不能隐式拒绝 Turn。

### `PreToolUse`

允许时，退出 `0` 且不输出内容。拒绝本次工具调用时：

```json
{
  "decision": "block",
  "reason": "This command is not allowed."
}
```

退出码 `2` 且将原因写入 stderr 是等价的拒绝形式。原因作为拒绝结果交给模型。其他
非零退出码表示 Hook 失败，不能隐式拒绝。本版本不支持改写工具参数。

### `Stop`

```json
{
  "systemMessage": "Turn audit saved."
}
```

公共 `Stop` 是观察与副作用 Hook。`decision: "block"`、`continue`、重试和自动创建
新输入不属于公共协议。

## 超出公共 Profile

格式正确但超出公共 profile 的内容不得使整个配置加载失败。

| 情况 | 统一模式行为 |
|---|---|
| 未知或非公共事件 | warning；不跨 Agent 执行 |
| 非 `command` handler | warning；不跨 Agent 执行 |
| 复杂 matcher | warning；不跨 Agent 执行对应 matcher group |
| 额外的纯元数据字段 | warning；公共适配器忽略 |
| 改变执行语义的字段，例如 `async` | warning；不跨 Agent 执行对应 handler |
| 公共事件中的非公共输出能力 | warning；忽略该能力，不影响同一输出中的公共字段 |

SessionMan 不实现通用 native passthrough。原生能力由原生 Provider 配置管理。跳过超出项时，
同一文件中的其他公共 Hook 仍然有效。

无效 JSON、缺少 `hooks`、空 `command`、非正数 `timeout` 等不是“超出公共 profile”，
而是无效配置，必须报错。

## Provider 适配

- Claude：使用原生事件名、配置结构和 command I/O。
- Codex：使用原生事件名、配置结构和 command I/O。
- Gemini：将四个公共事件映射到 `SessionStart`、`SessionEnd`、`BeforeAgent`、`AfterAgent`。
- agy：Hooks 与 MCP 的注入都要求指定 `C`；未指定 `C` 时不向 agy 注入任何内容。
- Pi：使用公开 ExtensionAPI；`tool_call` 实现 `PreToolUse` 的拒绝，`input` 负责 `UserPromptSubmit` 的拒绝，
  `before_agent_start` 注入其 `additionalContext`，`agent_settled` 实现公共 `Stop`。

适配器不得用一个新 Turn 模拟当前 Turn 的继续执行，也不得把原生扩展能力伪装成公共能力。

## 投影所有权

SessionMan 只从 `<C>/hooks.json` 向 Provider 生成单向投影。投影必须位于实现能够明确识别为
SessionMan-owned 的独立 artifact 中。

- 不修改或覆盖用户已有的原生 Hook。
- 不从 Provider artifact 反向同步规范配置。
- 不静态分析任意原生插件或扩展来补全统一 inventory。
- 无法安全创建独立 artifact 时必须报错，不能猜测合并。

预期用户体验见
[Unified Hook Implementation Example](unified-hook-implementation-example.md)。
