package mcp

import (
	"testing"

	"github.com/odigos-io/odigos/frontend/services/profiles/flamegraph"
)

func TestRankHotFunctions(t *testing.T) {
	symbols := []flamegraph.SymbolStats{
		{Name: "runtime.mallocgc", Self: 10, Total: 12},
		{Name: "main.processPayment", Self: 50, Total: 80},
		{Name: "net/http.(*conn).serve", Self: 5, Total: 100},
		{Name: "encoding/json.Marshal", Self: 35, Total: 40},
	}

	rows, totalSelf := rankHotFunctions(symbols, 2)

	if totalSelf != 100 {
		t.Fatalf("totalSelf = %d, want 100", totalSelf)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (top_n)", len(rows))
	}
	// Ranked by self time desc.
	if rows[0].Function != "main.processPayment" || rows[0].SelfSamples != 50 {
		t.Errorf("top function = %q (%d), want main.processPayment (50)", rows[0].Function, rows[0].SelfSamples)
	}
	if rows[0].SelfPercent != 50.0 {
		t.Errorf("top selfPercent = %v, want 50.0", rows[0].SelfPercent)
	}
	if rows[1].Function != "encoding/json.Marshal" {
		t.Errorf("second function = %q, want encoding/json.Marshal", rows[1].Function)
	}
}

func TestRankHotFunctionsEmpty(t *testing.T) {
	rows, total := rankHotFunctions(nil, 15)
	if rows != nil || total != 0 {
		t.Fatalf("empty input should yield (nil, 0), got (%v, %d)", rows, total)
	}
}

func TestRankHotFunctionsTopNClamped(t *testing.T) {
	symbols := []flamegraph.SymbolStats{
		{Name: "a", Self: 3, Total: 3},
		{Name: "b", Self: 1, Total: 1},
	}
	rows, _ := rankHotFunctions(symbols, 100)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (clamped to available)", len(rows))
	}
}
