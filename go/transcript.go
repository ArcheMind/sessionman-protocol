// Package transcript defines SessionMan Unified Transcript Protocol 2.0,
// shared by SessionMan and downstream consumers. The normative spec lives in
// spec/docs/unified-transcript-format.md and
// spec/schema/unified-transcript.schema.json.
package transcript

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ProtocolVersion = "2.0"

	SourceS1    = "s1"
	SourceS2    = "s2"
	SourceUser  = "user"
	SourceWorld = "world"

	MutabilityConstant = "constant"
	MutabilityCurrent  = "current"
	MutabilityDelta    = "delta"

	BlockText     = "text"
	BlockThinking = "thinking"
	BlockToolUse  = "tool_use"
	BlockImage    = "image"

	RuntimeSafetyRefusal  = "safety_refusal"
	RuntimeTurnAborted    = "turn_aborted"
	RuntimeQuotaExhausted = "quota_exhausted"
	RuntimeProviderError  = "provider_error"
)

type ContentBlock struct {
	Type      string `json:"type" yaml:"type"`
	Text      string `json:"text,omitempty" yaml:"text,omitempty"`
	Signature string `json:"signature,omitempty" yaml:"signature,omitempty"`
	ID        string `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Input     any    `json:"input,omitempty" yaml:"input,omitempty"`
	Source    any    `json:"source,omitempty" yaml:"source,omitempty"`
}

// serializedContentBlock preserves required text without adding it to other block types.
type serializedContentBlock struct {
	Type      string  `json:"type" yaml:"type"`
	Text      *string `json:"text,omitempty" yaml:"text,omitempty"`
	Signature string  `json:"signature,omitempty" yaml:"signature,omitempty"`
	ID        string  `json:"id,omitempty" yaml:"id,omitempty"`
	Name      string  `json:"name,omitempty" yaml:"name,omitempty"`
	Input     any     `json:"input,omitempty" yaml:"input,omitempty"`
	Source    any     `json:"source,omitempty" yaml:"source,omitempty"`
}

func (b ContentBlock) serialized() serializedContentBlock {
	var text *string
	if b.Type == BlockText || b.Text != "" {
		text = &b.Text
	}
	return serializedContentBlock{
		Type: b.Type, Text: text, Signature: b.Signature,
		ID: b.ID, Name: b.Name, Input: b.Input, Source: b.Source,
	}
}

func (b ContentBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.serialized())
}

func (b ContentBlock) MarshalYAML() (any, error) {
	return b.serialized(), nil
}

type ToolResult struct {
	ToolUseID string `json:"toolUseId" yaml:"toolUseId"`
	ToolName  string `json:"toolName" yaml:"toolName"`
	Result    any    `json:"result,omitempty" yaml:"result,omitempty"`
	IsError   bool   `json:"isError,omitempty" yaml:"isError,omitempty"`
}

type Producer struct {
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
	Origin   string `json:"origin,omitempty" yaml:"origin,omitempty"`
}

type Entry struct {
	ID                 string         `json:"id" yaml:"id"`
	ParentID           *string        `json:"parentId,omitempty" yaml:"parentId,omitempty"`
	Source             string         `json:"source" yaml:"source"`
	Mutability         string         `json:"mutability" yaml:"mutability"`
	Slot               string         `json:"slot,omitempty" yaml:"slot,omitempty"`
	Hash               string         `json:"hash,omitempty" yaml:"hash,omitempty"`
	SupersedesBeforeID string         `json:"supersedesBeforeId,omitempty" yaml:"supersedesBeforeId,omitempty"`
	Content            []ContentBlock `json:"content,omitempty" yaml:"content,omitempty"`
	ToolResult         *ToolResult    `json:"toolResult,omitempty" yaml:"toolResult,omitempty"`
	Producer           *Producer      `json:"producer,omitempty" yaml:"producer,omitempty"`
	StopReason         string         `json:"stopReason,omitempty" yaml:"stopReason,omitempty"`
	Timestamp          string         `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	RecordedAt         string         `json:"recordedAt,omitempty" yaml:"recordedAt,omitempty"`
}

// MarshalJSON keeps `content` present (possibly empty) on every entry without
// toolResult, and omits it on toolResult entries where the schema forbids it.
func (e Entry) MarshalJSON() ([]byte, error) {
	type plain Entry
	if e.ToolResult != nil {
		return json.Marshal(plain(e))
	}
	content := e.Content
	if content == nil {
		content = []ContentBlock{}
	}
	e.Content = nil
	return json.Marshal(struct {
		plain
		Content []ContentBlock `json:"content"`
	}{plain(e), content})
}

type RuntimeEvent struct {
	ID           string `json:"id" yaml:"id"`
	AfterEntryID string `json:"afterEntryId,omitempty" yaml:"afterEntryId,omitempty"`
	Provider     string `json:"provider" yaml:"provider"`
	Model        string `json:"model,omitempty" yaml:"model,omitempty"`
	Kind         string `json:"kind" yaml:"kind"`
	Code         string `json:"code,omitempty" yaml:"code,omitempty"`
	Message      string `json:"message,omitempty" yaml:"message,omitempty"`
	Timestamp    string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
}

type Session struct {
	ProtocolVersion string         `json:"protocolVersion" yaml:"protocolVersion"`
	ID              string         `json:"id" yaml:"id"`
	Provider        string         `json:"provider" yaml:"provider"`
	IsSubagent      bool           `json:"isSubagent,omitempty" yaml:"isSubagent,omitempty"`
	Workspace       string         `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Title           string         `json:"title,omitempty" yaml:"title,omitempty"`
	StartedAt       string         `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	UpdatedAt       string         `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
	EntryCount      int            `json:"entryCount" yaml:"entryCount"`
	Source          string         `json:"source,omitempty" yaml:"source,omitempty"`
	Entries         []Entry        `json:"entries" yaml:"entries"`
	RuntimeEvents   []RuntimeEvent `json:"runtimeEvents,omitempty" yaml:"runtimeEvents,omitempty"`
}

// MarshalJSON ensures entries serializes as [] instead of null when empty.
func (s Session) MarshalJSON() ([]byte, error) {
	type plain Session
	entries := s.Entries
	if entries == nil {
		entries = []Entry{}
	}
	s.Entries = nil
	return json.Marshal(struct {
		plain
		Entries []Entry `json:"entries"`
	}{plain(s), entries})
}

// TextContent joins all text blocks in an entry with newlines.
func TextContent(entry Entry) string {
	if entry.ToolResult != nil {
		return ""
	}
	parts := make([]string, 0, len(entry.Content))
	for _, b := range entry.Content {
		if b.Type == BlockText {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// ValidateSession enforces the 2.0 schema's structural rules plus entry-id and
// runtime-event-id uniqueness. Cross-entry referential checks (parentId,
// toolUseId, supersedesBeforeId) are intentionally not enforced.
func ValidateSession(session Session) error {
	var errs []error
	if session.ProtocolVersion != ProtocolVersion {
		errs = append(errs, fmt.Errorf("session: protocolVersion must be %q, got %q", ProtocolVersion, session.ProtocolVersion))
	}
	if session.ID == "" {
		errs = append(errs, errors.New("session: id must be non-empty"))
	}
	if session.Provider == "" {
		errs = append(errs, errors.New("session: provider must be non-empty"))
	}
	if session.EntryCount < 0 {
		errs = append(errs, errors.New("session: entryCount must be non-negative"))
	}
	if err := ValidateEntries(session.Entries); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateRuntimeEvents(session.RuntimeEvents); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ValidateEntries(entries []Entry) error {
	var errs []error
	seen := make(map[string]bool, len(entries))
	for i, entry := range entries {
		if entry.ID != "" {
			if seen[entry.ID] {
				errs = append(errs, fmt.Errorf("entries[%d]: duplicate id %q", i, entry.ID))
			}
			seen[entry.ID] = true
		}
		if err := ValidateEntry(entry); err != nil {
			errs = append(errs, fmt.Errorf("entries[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func ValidateEntry(entry Entry) error {
	var errs []error
	if entry.ID == "" {
		errs = append(errs, errors.New("id must be non-empty"))
	}

	switch entry.Mutability {
	case MutabilityConstant:
		if entry.Slot == "" {
			errs = append(errs, errors.New("mutability \"constant\" requires non-empty slot"))
		}
		if entry.SupersedesBeforeID != "" {
			errs = append(errs, errors.New("mutability \"constant\" must not carry supersedesBeforeId"))
		}
		if entry.ToolResult != nil {
			errs = append(errs, errors.New("mutability \"constant\" must not carry toolResult"))
		}
		if entry.StopReason != "" {
			errs = append(errs, errors.New("mutability \"constant\" must not carry stopReason"))
		}
	case MutabilityCurrent:
		if entry.Slot == "" {
			errs = append(errs, errors.New("mutability \"current\" requires non-empty slot"))
		}
		if entry.ToolResult != nil {
			errs = append(errs, errors.New("mutability \"current\" must not carry toolResult"))
		}
		if entry.StopReason != "" {
			errs = append(errs, errors.New("mutability \"current\" must not carry stopReason"))
		}
	case MutabilityDelta:
		if entry.Slot != "" {
			errs = append(errs, errors.New("mutability \"delta\" must not carry slot"))
		}
		if entry.Hash != "" {
			errs = append(errs, errors.New("mutability \"delta\" must not carry hash"))
		}
		if entry.SupersedesBeforeID != "" {
			errs = append(errs, errors.New("mutability \"delta\" must not carry supersedesBeforeId"))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown mutability %q", entry.Mutability))
	}

	switch entry.Source {
	case SourceS1:
		if entry.ToolResult != nil {
			errs = append(errs, errors.New("source \"s1\" must not carry toolResult"))
		}
		if entry.StopReason != "" {
			errs = append(errs, errors.New("source \"s1\" must not carry stopReason"))
		}
		for i, b := range entry.Content {
			if b.Type != BlockText {
				errs = append(errs, fmt.Errorf("content[%d]: source \"s1\" allows only text blocks, got %q", i, b.Type))
			}
		}
	case SourceS2:
		if entry.Mutability != "" && entry.Mutability != MutabilityDelta {
			errs = append(errs, fmt.Errorf("source \"s2\" requires mutability \"delta\", got %q", entry.Mutability))
		}
		if entry.ToolResult != nil {
			errs = append(errs, errors.New("source \"s2\" must not carry toolResult"))
		}
		for i, b := range entry.Content {
			if b.Type == BlockImage {
				errs = append(errs, fmt.Errorf("content[%d]: source \"s2\" does not allow image blocks", i))
			}
		}
	case SourceUser:
		if entry.ToolResult != nil {
			errs = append(errs, errors.New("source \"user\" must not carry toolResult"))
		}
		if entry.StopReason != "" {
			errs = append(errs, errors.New("source \"user\" must not carry stopReason"))
		}
		for i, b := range entry.Content {
			if b.Type != BlockText && b.Type != BlockImage {
				errs = append(errs, fmt.Errorf("content[%d]: source \"user\" allows only text/image blocks, got %q", i, b.Type))
			}
		}
	case SourceWorld:
		if entry.StopReason != "" {
			errs = append(errs, errors.New("source \"world\" must not carry stopReason"))
		}
		if entry.ToolResult != nil {
			if entry.Mutability != "" && entry.Mutability != MutabilityDelta {
				errs = append(errs, errors.New("toolResult requires mutability \"delta\""))
			}
			if len(entry.Content) > 0 {
				errs = append(errs, errors.New("world entry with toolResult must not carry content"))
			}
			if entry.ToolResult.ToolUseID == "" {
				errs = append(errs, errors.New("toolResult requires non-empty toolUseId"))
			}
			if entry.ToolResult.ToolName == "" {
				errs = append(errs, errors.New("toolResult requires non-empty toolName"))
			}
		} else {
			for i, b := range entry.Content {
				if b.Type != BlockText && b.Type != BlockImage {
					errs = append(errs, fmt.Errorf("content[%d]: source \"world\" allows only text/image blocks, got %q", i, b.Type))
				}
			}
		}
	default:
		errs = append(errs, fmt.Errorf("unknown source %q", entry.Source))
	}

	for i, block := range entry.Content {
		if err := validateBlock(block); err != nil {
			errs = append(errs, fmt.Errorf("content[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func ValidateRuntimeEvents(events []RuntimeEvent) error {
	var errs []error
	seen := make(map[string]bool, len(events))
	for i, ev := range events {
		if ev.ID == "" {
			errs = append(errs, fmt.Errorf("runtimeEvents[%d]: id must be non-empty", i))
		} else {
			if seen[ev.ID] {
				errs = append(errs, fmt.Errorf("runtimeEvents[%d]: duplicate id %q", i, ev.ID))
			}
			seen[ev.ID] = true
		}
		if ev.Provider == "" {
			errs = append(errs, fmt.Errorf("runtimeEvents[%d]: provider must be non-empty", i))
		}
		switch ev.Kind {
		case RuntimeSafetyRefusal, RuntimeTurnAborted, RuntimeQuotaExhausted, RuntimeProviderError:
		default:
			errs = append(errs, fmt.Errorf("runtimeEvents[%d]: unknown kind %q", i, ev.Kind))
		}
	}
	return errors.Join(errs...)
}

func validateBlock(block ContentBlock) error {
	switch block.Type {
	case BlockText:
		if block.ID != "" || block.Name != "" || block.Input != nil || block.Source != nil {
			return errors.New("text block must not carry id/name/input/source")
		}
		if block.Signature != "" {
			return errors.New("text block must not carry signature")
		}
		return nil
	case BlockThinking:
		if block.Name != "" || block.Input != nil || block.Source != nil {
			return errors.New("thinking block must not carry name/input/source")
		}
		return nil
	case BlockImage:
		if block.Source == nil {
			return errors.New("image block requires source")
		}
		if _, ok := block.Source.(map[string]any); !ok {
			return errors.New("image source must be an object")
		}
		if block.Text != "" || block.Signature != "" || block.ID != "" || block.Name != "" || block.Input != nil {
			return errors.New("image block must not carry text/signature/id/name/input")
		}
		return nil
	case BlockToolUse:
		if block.ID == "" || block.Name == "" {
			return errors.New("tool_use block requires id and name")
		}
		if block.Text != "" || block.Signature != "" || block.Source != nil {
			return errors.New("tool_use block must not carry text/signature/source")
		}
		if block.Input != nil {
			if _, ok := block.Input.(map[string]any); !ok {
				return errors.New("tool_use input must be an object")
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown block type %q", block.Type)
	}
}
