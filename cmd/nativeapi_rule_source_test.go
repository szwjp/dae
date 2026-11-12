/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The rule dictionary resolves a rule's file to the same source id the source
// list reports, and never publishes a path it should not.
func TestRuleSourceMapsToAConfigSource(t *testing.T) {
	dir := t.TempDir()
	provider := &nativeAPIProvider{config: &configEditor{
		baseDir:  dir,
		mainPath: filepath.Join(dir, "config.dae"),
	}}

	ruleFile := filepath.Join(dir, "config.d", "routing.dae")
	source := provider.ruleSource(ruleFile, 12)
	require.NotNil(t, source)
	require.Equal(t, "config.d/routing.dae", source.File)
	require.Equal(t, sourceIDFor("config.d/routing.dae"), source.SourceID)
	require.Equal(t, uint64(12), source.Line)

	// A file outside the configuration tree is not published at all.
	require.Nil(t, provider.ruleSource("/etc/passwd", 1))
	// The hidden api source stays hidden.
	require.Nil(t, provider.ruleSource(filepath.Join(dir, "api.dae"), 1))
	// A rule built in memory has no source.
	require.Nil(t, provider.ruleSource("", 0))
	require.Nil(t, provider.ruleSource(ruleFile, 0))
}
