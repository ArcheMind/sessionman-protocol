package agentx

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

func TestContentBlockRoundTrip(t *testing.T) {
	const input = `{
		"id":"empty-text", "provider":"test", "messageCount":3,
		"messages":[
			{"id":"assistant", "role":"assistant", "model":"test-model", "content":[
				{"type":"text", "text":""},
				{"type":"tool_use", "id":"call-1", "name":"read", "input":{"path":"file", "options":[true, 7]}},
				{"type":"text", "text":""},
				{"type":"thinking", "text":"reasoning", "signature":"signed"},
				{"type":"text", "text":""},
				{"type":"thinking", "signature":"opaque"},
				{"type":"text", "text":"done"},
				{"type":"text", "text":""}
			]},
			{"id":"user", "role":"user", "content":[
				{"type":"text", "text":""},
				{"type":"image", "source":{"type":"base64", "media_type":"image/png", "data":"abc"}},
				{"type":"text", "text":""}
			]},
			{"id":"system", "role":"system", "content":[{"type":"text", "text":""}]}
		]
	}`
	var original Session
	if err := json.Unmarshal([]byte(input), &original); err != nil {
		t.Fatal(err)
	}
	schema, err := jsonschema.Compile("../spec/schema/unified-transcript.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal([]byte(input), &want); err != nil {
		t.Fatal(err)
	}

	for _, format := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"JSON", json.Marshal, json.Unmarshal},
		{"YAML", yaml.Marshal, yaml.Unmarshal},
	} {
		t.Run(format.name, func(t *testing.T) {
			encoded, err := format.marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			// Check serialized fields before decoding into structs can hide omissions.
			var raw map[string]any
			if err := format.unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			normalized, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			var got any
			if err := json.Unmarshal(normalized, &got); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(got); err != nil {
				t.Fatalf("serialized session violates schema: %v\n%s", err, encoded)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("serialized fields or ordering changed:\ngot %s\nwant %s", normalized, input)
			}
			var restored Session
			if err := format.unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			var expected Session
			if err := format.unmarshal([]byte(input), &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored, expected) {
				t.Fatalf("round trip changed session:\ngot %#v\nwant %#v", restored, expected)
			}
			if err := ValidateSession(restored); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContentBlockSerializationPreservesOtherFields(t *testing.T) {
	// Invalid combinations must remain visible to validation, rather than lose data.
	original := ContentBlock{
		Type: BlockToolUse, Text: "unexpected", Signature: "signature",
		ID: "call-1", Name: "read", Input: map[string]any{"path": "file"},
		Source: map[string]any{"data": "image"},
	}
	for _, format := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"JSON", json.Marshal, json.Unmarshal},
		{"YAML", yaml.Marshal, yaml.Unmarshal},
	} {
		t.Run(format.name, func(t *testing.T) {
			encoded, err := format.marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var restored ContentBlock
			if err := format.unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored, original) {
				t.Fatalf("fields changed: got %#v, want %#v", restored, original)
			}
		})
	}
}
