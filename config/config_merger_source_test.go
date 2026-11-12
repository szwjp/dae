/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A rule has to keep the file and line it was written on: the native API's
// rule dictionary reports both, and a panel rewrites the rule in that file.
func TestMergeRecordsRuleSourceFileAndLine(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config.d")
	require.NoError(t, os.MkdirAll(configDir, 0o700))

	entry := filepath.Join(dir, "config.dae")
	require.NoError(t, os.WriteFile(entry, []byte(`include {
    config.d/*.dae
}
`), 0o600))

	routing := filepath.Join(configDir, "routing.dae")
	require.NoError(t, os.WriteFile(routing, []byte(`global {
  lan_interface: br-lan
}

routing {
  domain(geosite:cn) -> direct
  dip(geoip:private) -> direct
  fallback: direct
}
`), 0o600))

	sections, _, err := NewMerger(entry).Merge()
	require.NoError(t, err)
	conf, err := New(sections)
	require.NoError(t, err)

	require.Len(t, conf.Routing.Rules, 2)
	for i, wantLine := range []int{6, 7} {
		rule := conf.Routing.Rules[i]
		require.Equal(t, routing, rule.File, "rule %d file", i)
		require.Equal(t, wantLine, rule.Line, "rule %d line", i)
	}
}
