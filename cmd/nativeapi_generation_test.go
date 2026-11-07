/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import "testing"

// TestRuleDictionaryNamesTheConfigGeneration pins the alignment a client needs
// before it edits a rule: the rule list and GET /config must name the same
// generation, otherwise the client refuses to write (a source it is about to
// replace would belong to another generation than the list).
func TestRuleDictionaryNamesTheConfigGeneration(t *testing.T) {
	provider := &nativeAPIProvider{supervisor: newRuntimeSupervisor(&runtimeGeneration{id: "gen-abc"})}
	if got := provider.generationID("1"); got != "gen-abc" {
		t.Fatalf("generation = %q, want the supervisor's gen-abc", got)
	}

	// A generation that the supervisor does not have a name for keeps the
	// control plane's routing epoch rather than reporting nothing.
	provider.supervisor = newRuntimeSupervisor(&runtimeGeneration{})
	if got := provider.generationID("1"); got != "1" {
		t.Fatalf("generation = %q, want the routing epoch fallback 1", got)
	}

	// A provider without a supervisor still answers.
	provider.supervisor = nil
	if got := provider.generationID("1"); got != "1" {
		t.Fatalf("generation = %q, want 1", got)
	}
}
