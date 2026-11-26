# Proposal: temporality on input messages and a body-only system role

Status: **proposal, not normative.** The schema, implementations and fixtures
are unchanged. If accepted, the change lands as one commit per the repository
rule (spec + schema + Go + TypeScript + fixtures).

## Problem

Modern coding agents send the model far more than `system`, `user` and
`assistant` dialog. Their native sessions carry static directives,
environment snapshots, runtime notices, compaction summaries and recalled
memory, each in its own native slot. The unified transcript cannot say what
such a message is. Parsers either drop it or fold it into `user.text` /
`system.text`, and serializers cannot decide whether to restore it in another
agent or where to put it. In practice harness content has ended up as
visible `assistant` text, which presents it to the target model as the
model's own prior output.

## Model

Every input to the model comes from one of three sources, and is either
static (holds for the whole session) or dynamic (happened at a point in
time):

| | mind (the agent's own cognition) | body (the agent's carrier) | world (outside the agent) |
|---|---|---|---|
| **static** | Persona, values | Capabilities, permissions, modes, environment, vendor identity | Project instructions, long-term facts |
| **dynamic** | Recalled memory, summaries of its past | Interruptions, safety stops, errors, quota | User input, notices from other agents, tool results |

The model's own output (reply text, tool calls, its thinking) is not an
input and stays `assistant`.

The source axis maps onto existing roles, so it needs no new field:

| Role | Carries |
|---|---|
| `assistant` | The model's own output |
| `system` | **body only** |
| `user` | world, plus mind content injected by the transcript's owner |
| `tool` | world: tool results |

Mind content must not use `assistant`: in native thinking fields it is
invisible to the model, and in visible assistant text it presents injected
content as the model's own output.

## Proposal

1. Add one optional field to `Message`:

   ```jsonc
   { "role": "user", "temporality": "static",
     "content": [{ "type": "text", "text": "…" }] }
   ```

   `temporality` is `static` or `dynamic`, defaults to `dynamic`, and is
   forbidden on `assistant`.

2. Restrict `system` to body content. Parsers map each native input by its
   source, not by its native role: a vendor's self-description ("You are
   Codex, an agent based on GPT-6…") or `<environment_context>` becomes
   `system`; a subagent notification delivered through a native system
   channel becomes `user`.

## Cross-agent restore

When a session from agent X is resumed in agent Y:

| Message | Restore | Placement in Y |
|---|---|---|
| `system` | **drop** | — Y generates its own body. A `tool_use` left without a result by an interruption gets a synthetic interrupted result. |
| `user` static | restore | Y's harness channel at the start of the session |
| `user` dynamic | restore | In position, as user input |
| `tool` | restore | In position, as the tool result |
| `assistant` | restore | Text and tool calls as assistant output. Thinking: native if Y is the same model; otherwise see below |

Dropping `system` is what keeps one agent's system prompt out of another.
It also stops refusal loops: a Claude safety stop written back into history
as user text made every later turn refuse again; as `system` it is dropped.

Static `user` content is restored as its latest copy even when Y also loads
its own project instructions from the workspace; the two can differ when X
and Y run in different workspaces.

## Foreign thinking replay

Requirements: the target model must see it, it must not appear as the
target model's own output, and import must restore it losslessly to
`assistant.thinking`.

Native thinking fields do not work. With the signature or encrypted payload
removed, none of Claude Code, Codex, AGY or Pi exposed planted thinking text
to the model on native resume, while the same text in visible content was
recalled by all four.

Serializers write foreign thinking into the target's harness channel at the
position of the assistant message it belongs to, wrapped in a marker that
import lifts back into the referenced message:

```
<system-reminder>
<prior-reasoning of="<message id>" model="<model>">
…
</prior-reasoning>
</system-reminder>
```

Whether this placement avoids Claude Code safety refusals is unverified and
needs an A/B replay of a refused session.

## Message category matrix

Notation: `record.field` names the JSON level; `.type` values are written as
path segments. AGY names come from the `CORTEX_STEP_TYPE_*` enum and the
`cortex.proto` descriptors embedded in the binary. "not found" = no matching
step type in the schema; "no sample" = in the schema but absent from local
sessions. Versions: Claude Code 2.1.296, Codex 0.170.0, AGY 1.5.18,
sessionman-protocol v0.5.0.

| Class | Category | Content (local samples) | Claude Code | Codex | AGY | SM today | SM proposed | Cross-agent |
|---|---|---|---|---|---|---|---|---|
| **A. Static directives** | A1 Identity and rules | "You are Codex…", Claude's system prompt; owner-authored persona | `system` | `developer[0]` | not found | Claude: not persisted; Codex: `system.text` | vendor: `system`; persona: `user` static | vendor: drop; persona: restore |
| | A2 Capabilities | Skills, tools and MCP, subagents, plugins | `attachment.skill_listing` etc. | `developer` (`<tools>` etc.), `additional_tools` | not found | Claude: dropped; Codex: `system.text` | `system` | drop |
| | A3 Modes and permissions | Sandbox, collaboration mode, auto mode, multi-agent role | `attachment.auto_mode` etc. | `developer` (`<permissions instructions>` etc.) | not found | Claude: dropped; Codex: `system.text` | `system` | drop |
| | A4 Project instructions | AGENTS.md / CLAUDE.md | `attachment.instructions` | `user` (`# AGENTS.md instructions`) | `DIRECTORY_RULES` (no sample) | Claude, AGY: dropped; Codex: `user.text` | `user` static | restore latest |
| **B. Environment** | B1 Environment and session | cwd, date, model, account org | `attachment.environment` etc. | `user` (`<environment_context>`) | not found | Claude: dropped; Codex: `user.text` | `system` | drop |
| | B2 Hook context | SessionStart and other hook output | `attachment.hook_additional_context` | unknown | not found | Claude: dropped | by content, e.g. project index: `user` static | by role |
| **C. Runtime events** | C1 Async notices | Subagent / background task done, queued input | `user.isMeta`, `attachment.queued_command` | `user` (`<subagent_notification>`) | `SYSTEM_MESSAGE`, `USER_INPUT.is_queued_message`, `EPHEMERAL_MESSAGE` (no sample) | Claude: `user.text` (isMeta lost); Codex: `user.text`; AGY: `system.text` | `user` dynamic | restore |
| | C2 Interruptions and errors | Safety stop, aborted turn, quota exhausted | `user.isMeta`; text-bearing `system` record | `developer` (`<turn_aborted>`) | `ERROR_MESSAGE` | Claude: `user.text` / `system.text`; Codex: `system.text`; AGY: dropped | `system` | drop |
| | C3 Compaction summary | Summary replacing history | `user.isCompactSummary` | `compacted` | `CHECKPOINT` | Claude: `user.text` (flag lost); Codex, AGY: dropped | `user` dynamic | restore |
| **(recall)** | Recalled memory | Memories surfaced as background | `attachment.relevant_memories` | not found | `RETRIEVE_MEMORY` (no sample) | not parsed | `user` dynamic | restore |
| **D. Dialog** | D1 User input | What the human said | `user` | `user` | `USER_INPUT.query` | `user.text` | `user` (dynamic by default) | restore |
| | D2 Model reply | Visible answer | `assistant.text` | `assistant.output_text` | `PLANNER_RESPONSE.response` | `assistant.text` | unchanged | restore |
| | D3 Model reasoning | thinking / reasoning (encrypted) | `assistant.thinking` | `reasoning` | `PLANNER_RESPONSE.thinking_signature` | `assistant.thinking` | unchanged | native if same model, else marker |
| | D4 Tool calls and results | Calls and returned data | `assistant.tool_use`, `user.tool_result` | `function_call`, `function_call_output` | `PLANNER_RESPONSE.tool_calls`, tool steps / `GENERIC` | `assistant.tool_use`, `tool` | unchanged | restore |

## Serializer targets

| Message | Claude Code | Codex | AGY |
|---|---|---|---|
| `user` static | `user.isMeta` + `<system-reminder>` | `developer` | `SYSTEM_MESSAGE` |
| `user` dynamic | `user` | `user` | `USER_INPUT` |
| `tool` | `user.tool_result` | `function_call_output` | tool step |
| foreign thinking | `user.isMeta` + `<prior-reasoning>` | `developer` + `<prior-reasoning>` | `SYSTEM_MESSAGE` + `<prior-reasoning>` |

Unverified: whether hand-written Claude `attachment` or `isCompactSummary`
records load correctly, whether Codex `compacted` can be synthesized without
its encrypted replacement history, and whether AGY honors synthesized
`DIRECTORY_RULES`, `CHECKPOINT`, `EPHEMERAL_MESSAGE` or `RETRIEVE_MEMORY`
steps. The targets above use only slots observed in local native sessions.
