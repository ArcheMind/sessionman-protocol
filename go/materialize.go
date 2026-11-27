package transcript

type slotKey struct {
	source string
	slot   string
}

// Materialize projects a 2.0 session into the deterministic message sequence
// ready for cross-agent serialization:
//  1. Drops world×constant messages (target agents reload workspace rules from disk).
//  2. Deduplicates constant messages by (source, slot), keeping the latest value
//     in first-slot order.
//  3. Deduplicates current messages by (source, slot), keeping the latest value
//     in first-slot order; any current message carrying supersedesBeforeId trims
//     earlier delta messages and sits at the cut boundary.
//  4. Preserves surviving delta messages in causal order and closes any dangling
//     s2 tool_use with a synthetic interrupted world×delta toolResult.
//  5. Never includes runtimeEvents.
func Materialize(session Session) []Message {
	var constantOrder []slotKey
	constants := make(map[slotKey]Message)

	var currentOrder []slotKey
	currents := make(map[slotKey]Message)

	var deltas []Message

	for _, e := range session.Messages {
		switch e.Mutability {
		case MutabilityConstant:
			if e.Source == SourceWorld {
				continue // static world is always dropped on restore
			}
			k := slotKey{e.Source, e.Slot}
			if _, exists := constants[k]; !exists {
				constantOrder = append(constantOrder, k)
			}
			constants[k] = e
		case MutabilityCurrent:
			k := slotKey{e.Source, e.Slot}
			if _, exists := currents[k]; !exists {
				currentOrder = append(currentOrder, k)
			}
			currents[k] = e
		case MutabilityDelta:
			deltas = append(deltas, e)
		}
	}

	// Partition current messages into regular snapshots vs compaction boundaries.
	var regularCurrents []Message
	var boundaryCurrents []Message
	cutIndex := 0
	deltaIdx := make(map[string]int, len(deltas))
	for i, d := range deltas {
		deltaIdx[d.ID] = i
	}
	for _, k := range currentOrder {
		e := currents[k]
		if e.SupersedesBeforeID != "" {
			boundaryCurrents = append(boundaryCurrents, e)
			if idx, ok := deltaIdx[e.SupersedesBeforeID]; ok && idx > cutIndex {
				cutIndex = idx
			}
		} else {
			regularCurrents = append(regularCurrents, e)
		}
	}
	survivingDeltas := deltas[cutIndex:]

	// Collect matched toolUseIds in surviving deltas so dangling calls can be closed.
	resolvedTools := make(map[string]bool)
	for _, d := range survivingDeltas {
		if d.Source == SourceWorld && d.ToolResult != nil {
			resolvedTools[d.ToolResult.ToolUseID] = true
		}
	}

	out := make([]Message, 0, len(constantOrder)+len(regularCurrents)+len(boundaryCurrents)+len(survivingDeltas))
	for _, k := range constantOrder {
		out = append(out, constants[k])
	}
	out = append(out, regularCurrents...)
	out = append(out, boundaryCurrents...)

	for _, d := range survivingDeltas {
		out = append(out, d)
		if d.Source == SourceS2 {
			parent := d.ID
			for _, b := range d.Content {
				if b.Type == BlockToolUse && !resolvedTools[b.ID] {
					p := parent
					out = append(out, Message{
						ID:         "synthetic-interrupted-" + b.ID,
						ParentID:   &p,
						Source:     SourceWorld,
						Mutability: MutabilityDelta,
						ToolResult: &ToolResult{
							ToolUseID: b.ID,
							ToolName:  b.Name,
							Result:    "tool execution interrupted before completion",
							IsError:   true,
						},
					})
					resolvedTools[b.ID] = true
				}
			}
		}
	}

	return out
}
