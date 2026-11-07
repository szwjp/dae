/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"path/filepath"

	"github.com/daeuniverse/dae/component/nativeapi"
	"github.com/daeuniverse/dae/pkg/config_parser"
)

// ruleSource turns the location the parser kept into the contract's source
// member: the path and source_id the configuration API knows the file by, the
// 1-based line and, when known, the 1-based byte column.
//
// The location is only addressable when the provider can see the file: a
// statement from the hidden api.dae has no source in GET /config, and a
// configuration without an editor (or one that could not resolve its base
// directory) reports no location rather than one a client cannot open.
func (p *nativeAPIProvider) ruleSource(origin config_parser.Origin) *nativeapi.RuleSource {
	if p == nil || p.config == nil || origin.File == "" || origin.Line <= 0 {
		return nil
	}
	rel, err := filepath.Rel(p.config.baseDir, origin.File)
	if err != nil || rel == "." || filepath.IsAbs(rel) {
		return nil
	}
	rel = filepath.ToSlash(rel)
	// The hidden source is never addressable through the API, so pointing a
	// rule at it would hand the client a source it cannot read.
	if filepath.Base(rel) == hiddenConfigSourceName && rel != filepath.ToSlash(filepath.Base(p.config.mainPath)) {
		return nil
	}
	line := uint64(origin.Line)
	var column *uint64
	if origin.Column > 0 {
		col := uint64(origin.Column)
		column = &col
	}
	return &nativeapi.RuleSource{
		File:     rel,
		SourceID: sourceIDFor(rel),
		Line:     line,
		Column:   column,
	}
}
