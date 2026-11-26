# Proposal: `kind` on harness-injected messages

Status: **proposal, not normative.** The schema, implementations and fixtures
are unchanged. If accepted, the change lands as one commit per the repository
rule (spec + schema + Go + TypeScript + fixtures).

## Problem

Modern coding agents send the model far more than `system`, `user` and
`assistant` dialog. Their native sessions carry static directives,
environment snapshots, runtime notices, compaction summaries and recalled
memory, each in its own native slot. The unified transcript has no way to
say "this message was injected by the harness". Parsers either drop such
content or fold it into `user.text` / `system.text`, where it becomes
indistinguishable from real user input. Serializers then cannot pick the
target agent's native slot. In practice harness content has ended up as
visible `assistant` text, which presents it to the target model as the
model's own prior output.

## Proposal

Add one optional field to `Message`:

```json
{ "role": "user", "kind": "notice", "content": [{ "type": "text", "text": "…" }] }
```

| `kind` | Covers | Meaning |
|---|---|---|
| *(absent)* | D1–D4 | Dialog: real user input, model output, tool calls and results |
| `instructions` | A1–A4 | Static directives: identity, capabilities, modes, permissions, project rules |
| `context` | B1–B2 | Environment snapshots and hook-provided context |
| `notice` | C1–C2 | Runtime events: async completions, queued input, interruptions, errors |
| `summary` | C3 | Summary that replaces earlier history after compaction |
| `memory` | — | Recalled memory, background that reflects what was true when written |

Rules:

- `kind` is allowed on `user` and `system` only. It is forbidden on
  `assistant` and `tool`: harness content is never the model's own output.
- Absent `kind` keeps today's meaning, so existing transcripts stay valid.
- `role` keeps its wire meaning (which channel addresses the model); `kind`
  is orthogonal and says what the content is. Serializers choose the native
  slot from the pair.

Why not new roles: all three agents already deliver this content through
user, system or developer channels; the channel is not what is missing.
Why five values and not one per native type: the native granularity differs
(Codex distinguishes categories only by tags inside text), and cross-agent
replay only needs enough to choose a slot.

## Message category matrix

Notation: `record.field` names the JSON level; `.type` values are written as
path segments. AGY names come from the `CORTEX_STEP_TYPE_*` enum and the
`cortex.proto` descriptors embedded in the binary. "not found" = no matching
step type in the schema; "no sample" = in the schema but absent from local
sessions. Versions: Claude Code 2.1.296, Codex 0.170.0, AGY 1.5.18,
sessionman-protocol v0.5.0.

| Class | Category | Content (local samples) | Claude Code | Codex | AGY | SM today | SM proposed |
|---|---|---|---|---|---|---|---|
| **A. Static directives** | A1 Identity and rules | "You are Codex…", Claude's system prompt | `system` | `developer[0]` | not found | Claude: not persisted; Codex: `system.text` | `system{kind:instructions}` |
| | A2 Capabilities | Skills, tools and MCP, subagents, plugins | `attachment.skill_listing` etc. | `developer` (`<tools>` etc.), `additional_tools` | not found | Claude: dropped; Codex: `system.text` | `user\|system{kind:instructions}` |
| | A3 Modes and permissions | Sandbox, collaboration mode, auto mode, multi-agent role | `attachment.auto_mode` etc. | `developer` (`<permissions instructions>` etc.) | not found | Claude: dropped; Codex: `system.text` | `user\|system{kind:instructions}` |
| | A4 Project instructions | AGENTS.md / CLAUDE.md | `attachment.instructions` | `user` (`# AGENTS.md instructions`) | `DIRECTORY_RULES` (no sample) | Claude, AGY: dropped; Codex: `user.text` | `user{kind:instructions}` |
| **B. Environment** | B1 Environment and session | cwd, date, model, account org | `attachment.environment` etc. | `user` (`<environment_context>`) | not found | Claude: dropped; Codex: `user.text` | `user{kind:context}` |
| | B2 Hook context | SessionStart and other hook output | `attachment.hook_additional_context` | unknown | not found | Claude: dropped | `user{kind:context}` |
| **C. Runtime events** | C1 Async notices | Subagent / background task done, queued input | `user.isMeta`, `attachment.queued_command` | `user` (`<subagent_notification>`) | `SYSTEM_MESSAGE`, `USER_INPUT.is_queued_message`, `EPHEMERAL_MESSAGE` (no sample) | Claude: `user.text` (isMeta lost); Codex: `user.text`; AGY: `system.text` | `user\|system{kind:notice}` |
| | C2 Interruptions and errors | Safety stop, aborted turn, quota exhausted | `user.isMeta`; text-bearing `system` record | `developer` (`<turn_aborted>`) | `ERROR_MESSAGE` | Claude: `user.text` / `system.text`; Codex: `system.text`; AGY: dropped | `user\|system{kind:notice}` |
| | C3 Compaction summary | Summary replacing history | `user.isCompactSummary` | `compacted` | `CHECKPOINT` | Claude: `user.text` (flag lost); Codex, AGY: dropped | `user{kind:summary}` |
| **(recall)** | Recalled memory | Memories surfaced as background | `attachment.relevant_memories` | not found | `RETRIEVE_MEMORY` (no sample) | not parsed | `user{kind:memory}` |
| **D. Dialog** | D1 User input | What the human said | `user` | `user` | `USER_INPUT.query` | `user.text` | `user.text` (no `kind`) |
| | D2 Model reply | Visible answer | `assistant.text` | `assistant.output_text` | `PLANNER_RESPONSE.response` | `assistant.text` | unchanged |
| | D3 Model reasoning | thinking / reasoning (encrypted) | `assistant.thinking` | `reasoning` | `PLANNER_RESPONSE.thinking_signature` | `assistant.thinking` | unchanged |
| | D4 Tool calls and results | Calls and returned data | `assistant.tool_use`, `user.tool_result` | `function_call`, `function_call_output` | `PLANNER_RESPONSE.tool_calls`, tool steps / `GENERIC` | `assistant.tool_use`, `tool` | unchanged |

## Serializer targets

| `kind` | Claude Code | Codex | AGY |
|---|---|---|---|
| `instructions` | `user.isMeta` + `<system-reminder>` | `developer` | `SYSTEM_MESSAGE` |
| `context` | `user.isMeta` + `<system-reminder>` | `user` (tagged) | `SYSTEM_MESSAGE` |
| `notice` | `user.isMeta` + `<system-reminder>` | `developer` | `SYSTEM_MESSAGE` |
| `summary` | `user.isCompactSummary` | `user` (tagged) | `SYSTEM_MESSAGE` |
| `memory` | `user.isMeta` + `<system-reminder>` | `developer` | `SYSTEM_MESSAGE` |

Unverified: whether hand-written Claude `attachment` or `isCompactSummary`
records load correctly, whether Codex `compacted` can be synthesized without
its encrypted replacement history, and whether AGY honors synthesized
`DIRECTORY_RULES`, `CHECKPOINT`, `EPHEMERAL_MESSAGE` or `RETRIEVE_MEMORY`
steps. The targets above use only slots observed in local native sessions.

## Out of scope

- Foreign `thinking` replay. Local tests showed that none of Claude Code,
  Codex, AGY or Pi exposes unsigned imported thinking to the model.
- Whether `instructions` from one agent (e.g. Codex's own system prompt)
  should be replayed into another agent at all. `kind` makes the rule
  expressible; deciding it needs message origin, which this proposal does
  not add.
