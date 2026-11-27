// Unified transcript format 2.0 shared by SessionMan and downstream consumers.
// Normative spec: spec/docs/unified-transcript-format.md and
// spec/schema/unified-transcript.schema.json in this repository.

export const PROTOCOL_VERSION = "2.0" as const;

export type Source = "s1" | "s2" | "user" | "world";
export type Mutability = "constant" | "current" | "delta";
export type RuntimeEventKind =
  | "safety_refusal"
  | "turn_aborted"
  | "quota_exhausted"
  | "provider_error";

export interface TextBlock {
  type: "text";
  text: string;
}

export interface ThinkingBlock {
  type: "thinking";
  id?: string;
  text?: string;
  signature?: string;
}

export interface ToolUseBlock {
  type: "tool_use";
  id: string;
  name: string;
  input?: Record<string, unknown>;
}

export interface ImageBlock {
  type: "image";
  source: Record<string, unknown>;
}

export type ContentBlock = TextBlock | ImageBlock | ThinkingBlock | ToolUseBlock;

export interface ToolResult {
  toolUseId: string;
  toolName: string;
  result?: unknown;
  isError?: boolean;
}

export interface Producer {
  provider?: string;
  model?: string;
  origin?: string;
}

export interface Message {
  id: string;
  parentId?: string | null;
  source: Source;
  mutability: Mutability;
  slot?: string;
  hash?: string;
  supersedesBeforeId?: string;
  content?: ContentBlock[];
  toolResult?: ToolResult;
  producer?: Producer;
  stopReason?: string;
  timestamp?: string;
  recordedAt?: string;
}

export interface RuntimeEvent {
  id: string;
  afterMessageId?: string;
  provider: string;
  model?: string;
  kind: RuntimeEventKind;
  code?: string;
  message?: string;
  timestamp?: string;
}

export interface Session {
  protocolVersion: typeof PROTOCOL_VERSION;
  id: string;
  provider: string;
  isSubagent?: boolean;
  workspace?: string;
  title?: string;
  startedAt?: string;
  updatedAt?: string;
  messageCount?: number;
  source?: string;
  messages: Message[];
  runtimeEvents?: RuntimeEvent[];
}

// Legacy v0.x / v1 shapes for structural compatibility.
export interface V1Message {
  id: string;
  parentId?: string | null;
  role: "user" | "assistant" | "tool" | "system";
  content?: ContentBlock[];
  result?: unknown;
  timestamp?: string;
  recordedAt?: string;
  model?: string;
  stopReason?: string;
  toolUseId?: string;
  toolName?: string;
  isError?: boolean;
}

export interface V1Session {
  id: string;
  provider: string;
  isSubagent?: boolean;
  workspace?: string;
  title?: string;
  startedAt?: string;
  updatedAt?: string;
  messageCount?: number;
  source?: string;
  messages: V1Message[];
}

const VALID_SOURCES = new Set<string>(["s1", "s2", "user", "world"]);
const VALID_MUTABILITIES = new Set<string>(["constant", "current", "delta"]);
const VALID_RUNTIME_KINDS = new Set<string>([
  "safety_refusal",
  "turn_aborted",
  "quota_exhausted",
  "provider_error",
]);
const SESSION_KEYS = [
  "protocolVersion",
  "id",
  "provider",
  "isSubagent",
  "workspace",
  "title",
  "startedAt",
  "updatedAt",
  "messageCount",
  "source",
  "messages",
  "runtimeEvents",
];
const ENTRY_KEYS = [
  "id",
  "parentId",
  "source",
  "mutability",
  "slot",
  "hash",
  "supersedesBeforeId",
  "content",
  "toolResult",
  "producer",
  "stopReason",
  "timestamp",
  "recordedAt",
];

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const isNonEmptyString = (value: unknown): value is string =>
  typeof value === "string" && value.length > 0;

function forbidKeys(
  value: Record<string, unknown>,
  allowed: string[],
  problems: string[],
  prefix = "",
): void {
  for (const key of Object.keys(value)) {
    if (!allowed.includes(key)) {
      problems.push(`${prefix}unexpected key ${JSON.stringify(key)}`);
    }
  }
}

export function validateSession(value: unknown): string[] {
  if (!isRecord(value)) return ["session: must be an object"];
  const problems: string[] = [];
  forbidKeys(value, SESSION_KEYS, problems, "session: ");
  if (value.protocolVersion !== PROTOCOL_VERSION) {
    problems.push(`session: protocolVersion must be "${PROTOCOL_VERSION}"`);
  }
  if (!isNonEmptyString(value.id)) problems.push("session: id must be a non-empty string");
  if (!isNonEmptyString(value.provider)) problems.push("session: provider must be a non-empty string");
  if (
    value.messageCount !== undefined &&
    (typeof value.messageCount !== "number" || !Number.isInteger(value.messageCount) || value.messageCount < 0)
  ) {
    problems.push("session: messageCount must be a non-negative integer");
  }
  if (!Array.isArray(value.messages)) {
    problems.push("session: messages must be an array");
  } else {
    problems.push(...validateMessages(value.messages));
  }
  if (value.runtimeEvents !== undefined) {
    if (!Array.isArray(value.runtimeEvents)) {
      problems.push("session: runtimeEvents must be an array");
    } else {
      problems.push(...validateRuntimeEvents(value.runtimeEvents));
    }
  }
  return problems;
}

export function validateMessages(value: unknown): string[] {
  if (!Array.isArray(value)) return ["messages: must be an array"];
  const problems: string[] = [];
  const seen = new Set<string>();
  value.forEach((message, index) => {
    if (isRecord(message) && isNonEmptyString(message.id)) {
      if (seen.has(message.id)) {
        problems.push(`messages[${index}]: duplicate id ${JSON.stringify(message.id)}`);
      }
      seen.add(message.id);
    }
    problems.push(...validateMessage(message).map((p) => `messages[${index}]: ${p}`));
  });
  return problems;
}

export function validateMessage(value: unknown): string[] {
  if (!isRecord(value)) return ["must be an object"];
  const problems: string[] = [];
  forbidKeys(value, ENTRY_KEYS, problems);

  if (!isNonEmptyString(value.id)) problems.push("id must be a non-empty string");
  if (value.parentId !== undefined && value.parentId !== null && typeof value.parentId !== "string") {
    problems.push("parentId must be a string or null");
  }
  if (value.timestamp !== undefined && typeof value.timestamp !== "string") {
    problems.push("timestamp must be a string");
  }
  if (value.recordedAt !== undefined && typeof value.recordedAt !== "string") {
    problems.push("recordedAt must be a string");
  }

  const source = value.source;
  const mutability = value.mutability;
  if (typeof source !== "string" || !VALID_SOURCES.has(source)) {
    problems.push(`unknown source ${JSON.stringify(source)}`);
  }
  if (typeof mutability !== "string" || !VALID_MUTABILITIES.has(mutability)) {
    problems.push(`unknown mutability ${JSON.stringify(mutability)}`);
  }
  if (
    typeof source !== "string" ||
    !VALID_SOURCES.has(source) ||
    typeof mutability !== "string" ||
    !VALID_MUTABILITIES.has(mutability)
  ) {
    return problems;
  }

  if (mutability === "delta") {
    if (value.slot !== undefined) problems.push('mutability "delta" must not carry slot');
    if (value.hash !== undefined) problems.push('mutability "delta" must not carry hash');
    if (value.supersedesBeforeId !== undefined) {
      problems.push('mutability "delta" must not carry supersedesBeforeId');
    }
  } else {
    if (!isNonEmptyString(value.slot)) {
      problems.push(`mutability "${mutability}" requires non-empty slot`);
    }
    if (value.hash !== undefined && !isNonEmptyString(value.hash)) {
      problems.push("hash must be a non-empty string");
    }
    if (mutability === "constant" && value.supersedesBeforeId !== undefined) {
      problems.push('mutability "constant" must not carry supersedesBeforeId');
    }
    if (
      mutability === "current" &&
      value.supersedesBeforeId !== undefined &&
      !isNonEmptyString(value.supersedesBeforeId)
    ) {
      problems.push("supersedesBeforeId must be a non-empty string");
    }
    if (value.toolResult !== undefined) {
      problems.push(`mutability "${mutability}" must not carry toolResult`);
    }
    if (value.stopReason !== undefined) {
      problems.push(`mutability "${mutability}" must not carry stopReason`);
    }
  }

  if (value.producer !== undefined) {
    if (!isRecord(value.producer)) {
      problems.push("producer must be an object");
    } else {
      forbidKeys(value.producer, ["provider", "model", "origin"], problems, "producer: ");
      for (const k of ["provider", "model", "origin"] as const) {
        if (value.producer[k] !== undefined && !isNonEmptyString(value.producer[k])) {
          problems.push(`producer.${k} must be a non-empty string`);
        }
      }
    }
  }

  if (source === "s2") {
    if (mutability !== "delta") {
      problems.push(`source "s2" requires mutability "delta", got ${JSON.stringify(mutability)}`);
    }
    if (value.stopReason !== undefined && typeof value.stopReason !== "string") {
      problems.push("stopReason must be a string");
    }
  } else if (value.stopReason !== undefined) {
    problems.push(`source "${source}" must not carry stopReason`);
  }

  if (source === "world" && value.toolResult !== undefined) {
    if (value.content !== undefined) {
      problems.push("world message with toolResult must not carry content");
    }
    if (!isRecord(value.toolResult)) {
      problems.push("toolResult must be an object");
    } else {
      forbidKeys(value.toolResult, ["toolUseId", "toolName", "result", "isError"], problems, "toolResult: ");
      if (!isNonEmptyString(value.toolResult.toolUseId)) {
        problems.push("toolResult requires non-empty toolUseId");
      }
      if (!isNonEmptyString(value.toolResult.toolName)) {
        problems.push("toolResult requires non-empty toolName");
      }
      if (value.toolResult.isError !== undefined && typeof value.toolResult.isError !== "boolean") {
        problems.push("toolResult.isError must be a boolean");
      }
    }
  } else {
    if (value.toolResult !== undefined) {
      problems.push(`source "${source}" must not carry toolResult`);
    }
    if (!Array.isArray(value.content)) {
      problems.push("content must be an array");
    } else {
      value.content.forEach((block, index) => {
        problems.push(...validateBlock(block, source as Source).map((p) => `content[${index}]: ${p}`));
      });
    }
  }

  return problems;
}

export function validateRuntimeEvents(value: unknown): string[] {
  if (!Array.isArray(value)) return ["runtimeEvents: must be an array"];
  const problems: string[] = [];
  const seen = new Set<string>();
  value.forEach((ev, index) => {
    if (!isRecord(ev)) {
      problems.push(`runtimeEvents[${index}]: must be an object`);
      return;
    }
    forbidKeys(
      ev,
      ["id", "afterMessageId", "provider", "model", "kind", "code", "message", "timestamp"],
      problems,
      `runtimeEvents[${index}]: `,
    );
    if (!isNonEmptyString(ev.id)) {
      problems.push(`runtimeEvents[${index}]: id must be a non-empty string`);
    } else {
      if (seen.has(ev.id)) {
        problems.push(`runtimeEvents[${index}]: duplicate id ${JSON.stringify(ev.id)}`);
      }
      seen.add(ev.id);
    }
    if (!isNonEmptyString(ev.provider)) {
      problems.push(`runtimeEvents[${index}]: provider must be a non-empty string`);
    }
    if (typeof ev.kind !== "string" || !VALID_RUNTIME_KINDS.has(ev.kind)) {
      problems.push(`runtimeEvents[${index}]: unknown kind ${JSON.stringify(ev.kind)}`);
    }
  });
  return problems;
}

function validateBlock(value: unknown, source: Source): string[] {
  if (!isRecord(value)) return ["must be an object"];
  const problems: string[] = [];
  switch (value.type) {
    case "text":
      if (typeof value.text !== "string") problems.push("text block requires text string");
      forbidKeys(value, ["type", "text"], problems);
      break;
    case "image":
      if (source !== "user" && source !== "world") {
        problems.push(`source "${source}" does not allow image blocks`);
      }
      if (!isRecord(value.source)) problems.push("image block requires source object");
      forbidKeys(value, ["type", "source"], problems);
      break;
    case "thinking":
      if (source !== "s2") {
        problems.push(`source "${source}" does not allow thinking blocks`);
      }
      if (value.text !== undefined && typeof value.text !== "string") problems.push("thinking text must be a string");
      if (value.signature !== undefined && typeof value.signature !== "string") {
        problems.push("signature must be a string");
      }
      if (value.id !== undefined && typeof value.id !== "string") problems.push("thinking id must be a string");
      forbidKeys(value, ["type", "id", "text", "signature"], problems);
      break;
    case "tool_use":
      if (source !== "s2") {
        problems.push(`source "${source}" does not allow tool_use blocks`);
      }
      if (!isNonEmptyString(value.id)) problems.push("tool_use block requires id");
      if (!isNonEmptyString(value.name)) problems.push("tool_use block requires name");
      if (value.input !== undefined && !isRecord(value.input)) problems.push("tool_use input must be an object");
      forbidKeys(value, ["type", "id", "name", "input"], problems);
      break;
    default:
      problems.push(`unknown block type ${JSON.stringify(value.type)}`);
  }
  return problems;
}

export function materialize(session: Session): Message[] {
  const constantOrder: string[] = [];
  const constants = new Map<string, Message>();
  const currentOrder: string[] = [];
  const currents = new Map<string, Message>();
  const deltas: Message[] = [];

  for (const e of session.messages) {
    if (e.mutability === "constant") {
      if (e.source === "world") continue;
      const key = `${e.source}\0${e.slot}`;
      if (!constants.has(key)) constantOrder.push(key);
      constants.set(key, e);
    } else if (e.mutability === "current") {
      const key = `${e.source}\0${e.slot}`;
      if (!currents.has(key)) currentOrder.push(key);
      currents.set(key, e);
    } else {
      deltas.push(e);
    }
  }

  const regularCurrents: Message[] = [];
  const boundaryCurrents: Message[] = [];
  let cutIndex = 0;
  const deltaIdx = new Map<string, number>();
  deltas.forEach((d, idx) => deltaIdx.set(d.id, idx));

  for (const key of currentOrder) {
    const e = currents.get(key)!;
    if (e.supersedesBeforeId) {
      boundaryCurrents.push(e);
      const idx = deltaIdx.get(e.supersedesBeforeId);
      if (idx !== undefined && idx > cutIndex) cutIndex = idx;
    } else {
      regularCurrents.push(e);
    }
  }

  const survivingDeltas = deltas.slice(cutIndex);
  const resolvedTools = new Set<string>();
  for (const d of survivingDeltas) {
    if (d.source === "world" && d.toolResult) {
      resolvedTools.add(d.toolResult.toolUseId);
    }
  }

  const out: Message[] = [];
  for (const key of constantOrder) out.push(constants.get(key)!);
  out.push(...regularCurrents, ...boundaryCurrents);

  for (const d of survivingDeltas) {
    out.push(d);
    if (d.source === "s2" && d.content) {
      for (const b of d.content) {
        if (b.type === "tool_use" && !resolvedTools.has(b.id)) {
          out.push({
            id: `synthetic-interrupted-${b.id}`,
            parentId: d.id,
            source: "world",
            mutability: "delta",
            toolResult: {
              toolUseId: b.id,
              toolName: b.name,
              result: "tool execution interrupted before completion",
              isError: true,
            },
          });
          resolvedTools.add(b.id);
        }
      }
    }
  }
  return out;
}

export function upgradeFromV1(v1: V1Session): Session {
  const messages: Message[] = v1.messages.map((m, idx) => {
    const base: Partial<Message> = {
      id: m.id,
      mutability: "delta",
      ...(m.parentId !== undefined ? { parentId: m.parentId } : {}),
      ...(m.timestamp !== undefined ? { timestamp: m.timestamp } : {}),
      ...(m.recordedAt !== undefined ? { recordedAt: m.recordedAt } : {}),
    };
    switch (m.role) {
      case "user":
        return { ...base, source: "user", content: m.content ?? [] } as Message;
      case "tool":
        return {
          ...base,
          source: "world",
          toolResult: {
            toolUseId: m.toolUseId ?? "",
            toolName: m.toolName ?? "",
            ...(m.result !== undefined ? { result: m.result } : {}),
            ...(m.isError ? { isError: true } : {}),
          },
        } as Message;
      case "assistant": {
        const producer: Producer = {};
        if (v1.provider) producer.provider = v1.provider;
        if (m.model) producer.model = m.model;
        return {
          ...base,
          source: "s2",
          content: m.content ?? [],
          ...(Object.keys(producer).length > 0 ? { producer } : {}),
          ...(m.stopReason !== undefined ? { stopReason: m.stopReason } : {}),
        } as Message;
      }
      case "system":
        return { ...base, source: "s1", content: m.content ?? [] } as Message;
      default:
        throw new Error(`messages[${idx}]: unknown v1 role ${JSON.stringify(m.role)}`);
    }
  });

  const session: Session = {
    protocolVersion: PROTOCOL_VERSION,
    id: v1.id,
    provider: v1.provider,
    ...(v1.isSubagent !== undefined ? { isSubagent: v1.isSubagent } : {}),
    ...(v1.workspace !== undefined ? { workspace: v1.workspace } : {}),
    ...(v1.title !== undefined ? { title: v1.title } : {}),
    ...(v1.startedAt !== undefined ? { startedAt: v1.startedAt } : {}),
    ...(v1.updatedAt !== undefined ? { updatedAt: v1.updatedAt } : {}),
    messageCount: v1.messageCount ?? messages.length,
    ...(v1.source !== undefined ? { source: v1.source } : {}),
    messages,
  };
  const problems = validateSession(session);
  if (problems.length > 0) {
    throw new Error(`upgraded session invalid: ${problems.join("; ")}`);
  }
  return session;
}

export function parseSession(value: unknown): Session {
  if (!isRecord(value)) throw new Error("session: must be an object");
  if (value.protocolVersion === PROTOCOL_VERSION) {
    const problems = validateSession(value);
    if (problems.length > 0) {
      throw new Error(`invalid 2.0 session: ${problems.join("; ")}`);
    }
    return value as unknown as Session;
  }
  if (value.protocolVersion === undefined && Array.isArray(value.messages)) {
    return upgradeFromV1(value as unknown as V1Session);
  }
  throw new Error("session: unrecognized protocol version or envelope shape");
}

export const isTextBlock = (block: ContentBlock): block is TextBlock => block.type === "text";
export const isImageBlock = (block: ContentBlock): block is ImageBlock => block.type === "image";
export const isThinkingBlock = (block: ContentBlock): block is ThinkingBlock => block.type === "thinking";
export const isToolUseBlock = (block: ContentBlock): block is ToolUseBlock => block.type === "tool_use";

export const textContent = (message: Message): string =>
  (message.toolResult ? [] : (message.content ?? []))
    .filter(isTextBlock)
    .map((block) => block.text)
    .join("\n");
