// SPDX-License-Identifier: GPL-2.0-or-later
package hub

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Exports the exact assistant contract for harness comparisons (D02).
// MUTTI_DUMP_CONTRACT=/path/contract.json go test -run TestDumpContract
func TestDumpContract(t *testing.T) {
	path := os.Getenv("MUTTI_DUMP_CONTRACT")
	if path == "" {
		t.Skip("set MUTTI_DUMP_CONTRACT")
	}
	defs := append(append(movieTools(), documentTools()...), photoTools()...)
	b, _ := json.MarshalIndent(map[string]any{"prompt": PromptVersion, "tools": defs,
		"system": SystemPrompt("Qwen 3.5 4B", time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), defs)}, "", " ")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
