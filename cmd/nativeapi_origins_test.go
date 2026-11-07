/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/daeuniverse/dae/pkg/config_parser"
)

func TestRuleSourceMapsOriginToConfigSource(t *testing.T) {
	base := t.TempDir()
	provider := &nativeAPIProvider{config: &configEditor{
		mainPath: filepath.Join(base, "config.dae"),
		baseDir:  base,
	}}
	origin := config_parser.Origin{
		File:   filepath.Join(base, "config.d", "routing.dae"),
		Line:   15,
		Column: 1,
	}
	source := provider.ruleSource(origin)
	if source == nil {
		t.Fatal("source = nil")
	}
	if source.File != "config.d/routing.dae" {
		t.Fatalf("file = %q", source.File)
	}
	if source.SourceID != sourceIDFor("config.d/routing.dae") {
		t.Fatalf("source_id = %q", source.SourceID)
	}
	if source.Line != 15 {
		t.Fatalf("line = %d", source.Line)
	}
	if source.Column == nil || *source.Column != 1 {
		t.Fatalf("column = %#v", source.Column)
	}

	// The hidden source is not addressable through the API.
	hidden := provider.ruleSource(config_parser.Origin{File: filepath.Join(base, "config.d", "api.dae"), Line: 3})
	if hidden != nil {
		t.Fatalf("hidden source reported = %#v", hidden)
	}
	// A rule the engine built itself, or one without an editor, has no location.
	if got := provider.ruleSource(config_parser.Origin{}); got != nil {
		t.Fatalf("empty origin reported = %#v", got)
	}
	provider.config = nil
	if got := provider.ruleSource(origin); got != nil {
		t.Fatalf("source without an editor = %#v", got)
	}
}
