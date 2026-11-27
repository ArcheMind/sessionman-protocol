package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

func fixturePaths(t *testing.T, kind string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "spec", "fixtures", kind, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s fixtures found", kind)
	}
	return paths
}

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	s, err := jsonschema.Compile("../spec/schema/unified-transcript.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidFixtures(t *testing.T) {
	schema := compileSchema(t)
	for _, path := range fixturePaths(t, "valid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var raw any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("unmarshal raw: %v", err)
			}
			if err := schema.Validate(raw); err != nil {
				t.Fatalf("schema rejected valid fixture: %v", err)
			}
			session, err := ParseSession(data)
			if err != nil {
				t.Fatalf("ParseSession rejected valid fixture: %v", err)
			}
			if err := ValidateSession(session); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			yamlBytes, err := yaml.Marshal(session)
			if err != nil {
				t.Fatalf("yaml marshal: %v", err)
			}
			fromYAML, err := ParseSession(yamlBytes)
			if err != nil {
				t.Fatalf("ParseSession rejected YAML round trip: %v\n%s", err, yamlBytes)
			}
			if fromYAML.ID != session.ID || len(fromYAML.Messages) != len(session.Messages) {
				t.Fatalf("YAML round trip mismatch: got %+v", fromYAML)
			}
		})
	}
}

func TestInvalidFixtures(t *testing.T) {
	schema := compileSchema(t)
	for _, path := range fixturePaths(t, "invalid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var wrapper struct {
				Reason  string          `json:"reason"`
				Session json.RawMessage `json:"session"`
			}
			if err := json.Unmarshal(data, &wrapper); err != nil {
				t.Fatalf("fixture wrapper: %v", err)
			}
			if wrapper.Reason == "" {
				t.Fatal("invalid fixture must state a reason")
			}
			var raw any
			if err := json.Unmarshal(wrapper.Session, &raw); err == nil {
				if err := schema.Validate(raw); err == nil {
					t.Fatalf("schema unexpectedly accepted invalid fixture (%s)", wrapper.Reason)
				}
			}
			var session Session
			if err := json.Unmarshal(wrapper.Session, &session); err != nil {
				return
			}
			if err := ValidateSession(session); err == nil {
				t.Fatalf("validator unexpectedly accepted invalid fixture (%s)", wrapper.Reason)
			}
		})
	}
}

func TestV1UpgradeFixtures(t *testing.T) {
	schema := compileSchema(t)
	for _, path := range fixturePaths(t, "v1") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			upgraded, err := ParseSession(data)
			if err != nil {
				t.Fatalf("ParseSession failed on v1 fixture: %v", err)
			}
			if upgraded.ProtocolVersion != ProtocolVersion {
				t.Fatalf("expected protocolVersion %q, got %q", ProtocolVersion, upgraded.ProtocolVersion)
			}
			encoded, err := json.Marshal(upgraded)
			if err != nil {
				t.Fatal(err)
			}
			var raw any
			if err := json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(raw); err != nil {
				t.Fatalf("upgraded v1 session failed 2.0 schema: %v\n%s", err, encoded)
			}
		})
	}
}

// ToolResult messages must serialize without "content"; every other message must
// keep it, even when empty, since the schema requires it there.
func TestMarshalContentPresence(t *testing.T) {
	for _, path := range fixturePaths(t, "valid") {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			t.Fatal(err)
		}
		for _, message := range session.Messages {
			out, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(out, &raw); err != nil {
				t.Fatal(err)
			}
			if _, has := raw["content"]; has == (message.ToolResult != nil) {
				t.Fatalf("%s: message %s: content presence wrong in %s", path, message.ID, out)
			}
		}
	}
}

func TestMaterialize(t *testing.T) {
	data, err := os.ReadFile("../spec/fixtures/valid/full-20.json")
	if err != nil {
		t.Fatal(err)
	}
	session, err := ParseSession(data)
	if err != nil {
		t.Fatal(err)
	}

	got := Materialize(session)
	// Expected:
	// - id-1 (s1 constant)
	// - rules-1 (world constant) DROPPED
	// - board-2 (s1 current, supersedes board-1)
	// - summary-1 (s1 current with supersedesBeforeId=w-notice-1, trims u1/a1/t1)
	// - w-notice-1, s1-hook-1, a2
	wantIDs := []string{"id-1", "board-2", "summary-1", "w-notice-1", "s1-hook-1", "a2"}
	if len(got) != len(wantIDs) {
		gotIDs := make([]string, len(got))
		for i, e := range got {
			gotIDs[i] = e.ID
		}
		t.Fatalf("Materialize ids = %v, want %v", gotIDs, wantIDs)
	}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Fatalf("Materialize[%d].ID = %q, want %q", i, got[i].ID, want)
		}
	}

	// Dangling tool_use receives a synthetic interrupted world×delta toolResult.
	dangling := Session{
		ProtocolVersion: ProtocolVersion,
		ID:              "sess-dangling",
		Provider:        "claude",
		Messages: []Message{
			{
				ID:         "a1",
				Source:     SourceS2,
				Mutability: MutabilityDelta,
				Content: []ContentBlock{
					{Type: BlockToolUse, ID: "call-orphan", Name: "Bash", Input: map[string]any{"cmd": "ls"}},
				},
			},
		},
		RuntimeEvents: []RuntimeEvent{
			{ID: "re-1", AfterMessageID: "a1", Provider: "claude", Kind: RuntimeTurnAborted},
		},
	}
	mat := Materialize(dangling)
	if len(mat) != 2 {
		t.Fatalf("expected synthetic toolResult for dangling call, got %d messages", len(mat))
	}
	if mat[1].Source != SourceWorld || mat[1].Mutability != MutabilityDelta || mat[1].ToolResult == nil {
		t.Fatalf("unexpected synthetic message: %+v", mat[1])
	}
	if mat[1].ToolResult.ToolUseID != "call-orphan" || !mat[1].ToolResult.IsError {
		t.Fatalf("unexpected synthetic toolResult: %+v", mat[1].ToolResult)
	}
}
