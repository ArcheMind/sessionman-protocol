# Proposal: source and temporality on input messages

Status: **proposal, not normative.** The schema, implementations and fixtures
are unchanged. If accepted, the change lands as one commit per the repository
rule (spec + schema + Go + TypeScript + fixtures).

## Problem

Modern coding agents send the model far more than `system`, `user` and
`assistant` dialog. Their native sessions carry static directives,
environment snapshots, runtime notices, compaction summaries and recalled
memory, each in its own native slot. The unified transcript cannot say what
such a message is. Parsers either drop it or fold it into `user.text` /
`system.text`, where it becomes indistinguishable from real user input, and
serializers cannot decide whether to restore it in another agent or where to
put it. In practice harness content has ended up as visible `assistant` text,
which presents it to the target model as the model's own prior output.

## Proposal

Add two optional fields to `Message`. `role` is unchanged.

```jsonc
{ "role": "system", "source": "mind", "temporality": "static",
  "content": [{ "type": "text", "text": "…" }] }
```

| Field | Values | Meaning |
|---|---|---|
| `source` | `mind` | The agent's own cognition: identity, recalled memory, summaries of its past |
| | `body` | The agent's carrier: capabilities, permissions, modes, environment, interruptions, errors, quota |
| | `world` | Outside the agent: users, other agents, project documents, tool observations |
| `temporality` | `static` | Holds for the whole session; a newer version supersedes an older one |
| | `dynamic` | Happened at a point in time; order matters |

Rules:

- `assistant` messages carry neither field. They are the model's own output.
- `user` and `tool` messages default to `world` + `dynamic` when the fields
  are absent, so today's dialog keeps its meaning.
- `system` messages must carry both fields.
- Classify by where the content comes from, not by the delivery mechanism. A
  hook that injects a project index delivers `world` + `static` content.
- A vendor's self-description of its carrier ("You are Codex, an agent based
  on GPT-6…") is `body` + `static`. A persona authored by the transcript's
  owner is `mind` + `static`.

## Cross-agent restore

When a session from agent X is resumed in agent Y, the fields alone decide
what is restored:

```
            mind       body                        world
static      restore    drop                        open question
dynamic     restore    drop (close open tool_use)  restore, keep order
```

- **body** is dropped: Y regenerates its own capabilities, permissions,
  environment and identity, and X's interruptions, refusals and errors
  describe a body that is no longer running. Replaying them caused a loop in
  practice: a Claude safety refusal written back as a user message made every
  later turn refuse again. A `tool_use` left without a result by an
  interruption gets a synthetic interrupted result so the history stays
  paired.
- **mind** and **world dynamic** are restored: they are the continuity of
  the self and of its interaction with the world.
- **assistant** messages are restored. Thinking from a different model is
  replayed through the target's harness channel (below), not as assistant
  text and not as a native thinking block.
- **world static** is open: drop it and let Y re-read its own project
  instructions from the workspace, or restore the latest copy. They differ
  when X and Y run in different workspaces.

## Foreign thinking replay

Requirements: the target model must see it, it must not appear as the
target model's own output, and import must restore it losslessly to
`assistant.thinking`.

Native thinking fields do not work. With the signature or encrypted payload
removed, none of Claude Code, Codex, AGY or Pi exposed planted thinking text
to the model on native resume, while the same text in visible content was
recalled by all four. Visible `assistant` text works but presents another
model's reasoning as the target's own output.

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

Same-model signed thinking keeps its native channel. Whether this placement
avoids Claude Code safety refusals is unverified and needs an A/B replay of a
refused session.

## Message category matrix

Notation: `record.field` names the JSON level; `.type` values are written as
path segments. AGY names come from the `CORTEX_STEP_TYPE_*` enum and the
`cortex.proto` descriptors embedded in the binary. "not found" = no matching
step type in the schema; "no sample" = in the schema but absent from local
sessions. Versions: Claude Code 2.1.296, Codex 0.170.0, AGY 1.5.18,
sessionman-protocol v0.5.0.

| Class | Category | Content (local samples) | Claude Code | Codex | AGY | SM today | SM proposed | Cross-agent |
|---|---|---|---|---|---|---|---|---|
| **A. Static directives** | A1 Identity and rules | "You are Codex…", Claude's system prompt; owner-authored persona | `system` | `developer[0]` | not found | Claude: not persisted; Codex: `system.text` | vendor: `system` body·static; persona: `system` mind·static | vendor: drop; persona: restore |
| | A2 Capabilities | Skills, tools and MCP, subagents, plugins | `attachment.skill_listing` etc. | `developer` (`<tools>` etc.), `additional_tools` | not found | Claude: dropped; Codex: `system.text` | `system` body·static | drop |
| | A3 Modes and permissions | Sandbox, collaboration mode, auto mode, multi-agent role | `attachment.auto_mode` etc. | `developer` (`<permissions instructions>` etc.) | not found | Claude: dropped; Codex: `system.text` | `system` body·static | drop |
| | A4 Project instructions | AGENTS.md / CLAUDE.md | `attachment.instructions` | `user` (`# AGENTS.md instructions`) | `DIRECTORY_RULES` (no sample) | Claude, AGY: dropped; Codex: `user.text` | `system` world·static | open |
| **B. Environment** | B1 Environment and session | cwd, date, model, account org | `attachment.environment` etc. | `user` (`<environment_context>`) | not found | Claude: dropped; Codex: `user.text` | `system` body·static | drop |
| | B2 Hook context | SessionStart and other hook output | `attachment.hook_additional_context` | unknown | not found | Claude: dropped | by content, e.g. project index: `system` world·static | by cell |
| **C. Runtime events** | C1 Async notices | Subagent / background task done, queued input | `user.isMeta`, `attachment.queued_command` | `user` (`<subagent_notification>`) | `SYSTEM_MESSAGE`, `USER_INPUT.is_queued_message`, `EPHEMERAL_MESSAGE` (no sample) | Claude: `user.text` (isMeta lost); Codex: `user.text`; AGY: `system.text` | `user` world·dynamic | restore |
| | C2 Interruptions and errors | Safety stop, aborted turn, quota exhausted | `user.isMeta`; text-bearing `system` record | `developer` (`<turn_aborted>`) | `ERROR_MESSAGE` | Claude: `user.text` / `system.text`; Codex: `system.text`; AGY: dropped | `system` body·dynamic | drop |
| | C3 Compaction summary | Summary replacing history | `user.isCompactSummary` | `compacted` | `CHECKPOINT` | Claude: `user.text` (flag lost); Codex, AGY: dropped | `system` mind·dynamic | restore |
| **(recall)** | Recalled memory | Memories surfaced as background | `attachment.relevant_memories` | not found | `RETRIEVE_MEMORY` (no sample) | not parsed | `system` mind·dynamic | restore |
| **D. Dialog** | D1 User input | What the human said | `user` | `user` | `USER_INPUT.query` | `user.text` | `user` (world·dynamic by default) | restore |
| | D2 Model reply | Visible answer | `assistant.text` | `assistant.output_text` | `PLANNER_RESPONSE.response` | `assistant.text` | unchanged | restore |
| | D3 Model reasoning | thinking / reasoning (encrypted) | `assistant.thinking` | `reasoning` | `PLANNER_RESPONSE.thinking_signature` | `assistant.thinking` | unchanged | native if same model, else marker |
| | D4 Tool calls and results | Calls and returned data | `assistant.tool_use`, `user.tool_result` | `function_call`, `function_call_output` | `PLANNER_RESPONSE.tool_calls`, tool steps / `GENERIC` | `assistant.tool_use`, `tool` | unchanged (`tool` world·dynamic by default) | restore |

## Serializer targets for restored input

| Cell | Claude Code | Codex | AGY |
|---|---|---|---|
| mind·static | `user.isMeta` + `<system-reminder>` | `developer` | `SYSTEM_MESSAGE` |
| mind·dynamic | `user.isMeta` + `<system-reminder>`, in position | `developer`, in position | `SYSTEM_MESSAGE`, in position |
| world·dynamic | `user` / `user.tool_result` | `user` / `function_call_output` | `USER_INPUT` / tool step |
| world·static | open | open | open |

Unverified: whether hand-written Claude `attachment` or `isCompactSummary`
records load correctly, whether Codex `compacted` can be synthesized without
its encrypted replacement history, and whether AGY honors synthesized
`DIRECTORY_RULES`, `CHECKPOINT`, `EPHEMERAL_MESSAGE` or `RETRIEVE_MEMORY`
steps. The targets above use only slots observed in local native sessions.
