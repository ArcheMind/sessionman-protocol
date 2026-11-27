package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// V1Message represents the legacy v0.x/v1 message shape (role-based).
type V1Message struct {
	ID         string         `json:"id" yaml:"id"`
	ParentID   *string        `json:"parentId,omitempty" yaml:"parentId,omitempty"`
	Role       string         `json:"role" yaml:"role"`
	Content    []ContentBlock `json:"content,omitempty" yaml:"content,omitempty"`
	Result     any            `json:"result,omitempty" yaml:"result,omitempty"`
	Timestamp  string         `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`
	RecordedAt string         `json:"recordedAt,omitempty" yaml:"recordedAt,omitempty"`
	Model      string         `json:"model,omitempty" yaml:"model,omitempty"`
	StopReason string         `json:"stopReason,omitempty" yaml:"stopReason,omitempty"`
	ToolUseID  string         `json:"toolUseId,omitempty" yaml:"toolUseId,omitempty"`
	ToolName   string         `json:"toolName,omitempty" yaml:"toolName,omitempty"`
	IsError    bool           `json:"isError,omitempty" yaml:"isError,omitempty"`
}

// V1Session represents the legacy v0.x/v1 session envelope.
type V1Session struct {
	ID           string      `json:"id" yaml:"id"`
	Provider     string      `json:"provider" yaml:"provider"`
	IsSubagent   bool        `json:"isSubagent,omitempty" yaml:"isSubagent,omitempty"`
	Workspace    string      `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Title        string      `json:"title,omitempty" yaml:"title,omitempty"`
	StartedAt    string      `json:"startedAt,omitempty" yaml:"startedAt,omitempty"`
	UpdatedAt    string      `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
	MessageCount int         `json:"messageCount" yaml:"messageCount"`
	Source       string      `json:"source,omitempty" yaml:"source,omitempty"`
	Messages     []V1Message `json:"messages" yaml:"messages"`
}

// UpgradeFromV1 performs a pure structural conversion from a v1 session to 2.0
// without inspecting or rewriting message content.
func UpgradeFromV1(v1 V1Session) (Session, error) {
	messages := make([]Message, 0, len(v1.Messages))
	for i, m := range v1.Messages {
		message := Message{
			ID:         m.ID,
			ParentID:   m.ParentID,
			Mutability: MutabilityDelta,
			Timestamp:  m.Timestamp,
			RecordedAt: m.RecordedAt,
		}
		switch m.Role {
		case "user":
			message.Source = SourceUser
			message.Content = m.Content
		case "tool":
			message.Source = SourceWorld
			message.ToolResult = &ToolResult{
				ToolUseID: m.ToolUseID,
				ToolName:  m.ToolName,
				Result:    m.Result,
				IsError:   m.IsError,
			}
		case "assistant":
			message.Source = SourceS2
			message.Content = m.Content
			message.StopReason = m.StopReason
			if v1.Provider != "" || m.Model != "" {
				message.Producer = &Producer{
					Provider: v1.Provider,
					Model:    m.Model,
				}
			}
		case "system":
			message.Source = SourceS1
			message.Content = m.Content
		default:
			return Session{}, fmt.Errorf("messages[%d]: unknown v1 role %q", i, m.Role)
		}
		messages = append(messages, message)
	}
	count := v1.MessageCount
	if count == 0 && len(messages) > 0 {
		count = len(messages)
	}
	out := Session{
		ProtocolVersion: ProtocolVersion,
		ID:              v1.ID,
		Provider:        v1.Provider,
		IsSubagent:      v1.IsSubagent,
		Workspace:       v1.Workspace,
		Title:           v1.Title,
		StartedAt:       v1.StartedAt,
		UpdatedAt:       v1.UpdatedAt,
		MessageCount:      count,
		Source:          v1.Source,
		Messages:         messages,
	}
	if err := ValidateSession(out); err != nil {
		return Session{}, err
	}
	return out, nil
}

// ParseSession decodes either a Protocol 2.0 session or a legacy v1 session
// (automatically upgraded via UpgradeFromV1) in JSON or YAML format, and
// validates the resulting 2.0 session.
func ParseSession(data []byte) (Session, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Session{}, errors.New("session: empty input")
	}
	if trimmed[0] == '{' {
		return parseSessionWith(trimmed, json.Unmarshal)
	}
	return parseSessionWith(trimmed, yaml.Unmarshal)
}

func parseSessionWith(data []byte, unmarshal func([]byte, any) error) (Session, error) {
	var probe struct {
		ProtocolVersion string `json:"protocolVersion" yaml:"protocolVersion"`
	}
	if err := unmarshal(data, &probe); err != nil {
		return Session{}, err
	}
	// Distinguish explicit empty array vs absent key via raw map probe.
	var raw map[string]any
	if err := unmarshal(data, &raw); err != nil {
		return Session{}, err
	}
	_, hasMessages := raw["messages"]

	if probe.ProtocolVersion == ProtocolVersion {
		var s Session
		if err := unmarshal(data, &s); err != nil {
			return Session{}, err
		}
		if err := ValidateSession(s); err != nil {
			return Session{}, err
		}
		return s, nil
	}
	if probe.ProtocolVersion == "" && hasMessages {
		var v1 V1Session
		if err := unmarshal(data, &v1); err != nil {
			return Session{}, err
		}
		return UpgradeFromV1(v1)
	}
	return Session{}, errors.New("session: unrecognized protocol version or envelope shape")
}
