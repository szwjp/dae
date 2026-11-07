/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/daeuniverse/dae/pkg/config_parser"
)

// TestMergedConfigKeepsOrigins is the end-to-end check of the source
// locations: the merger parses each file on its own, the rules of the merged
// configuration point back at the file and line they were written on, and the
// fallback declarations inside the dns block are located too.
func TestMergedConfigKeepsOrigins(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "config.dae")
	if err := os.MkdirAll(filepath.Join(dir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(`include {
	config.d/*.dae
}
global {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	routingPath := filepath.Join(dir, "config.d", "routing.dae")
	routing := `routing {
	pname(curl) && dport(443) -> proxy
	mac('00:e0:4c:2c:1a:b5') -> must_direct
	fallback: direct
}
`
	if err := os.WriteFile(routingPath, []byte(routing), 0o600); err != nil {
		t.Fatal(err)
	}
	dnsPath := filepath.Join(dir, "config.d", "dns.dae")
	dns := `dns {
	upstream {
		alidns: 'udp://223.5.5.5:53'
	}
	routing {
		request {
			qname(suffix: example.com) -> alidns
			fallback: asis
		}
		response {
			fallback: accept
		}
	}
}
`
	if err := os.WriteFile(dnsPath, []byte(dns), 0o600); err != nil {
		t.Fatal(err)
	}

	sections, _, err := NewMerger(entry).Merge()
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	conf, err := New(sections)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if got := len(conf.Routing.Rules); got != 2 {
		t.Fatalf("rules = %d, want 2", got)
	}
	first := conf.Routing.Rules[0]
	if first.Origin.File != routingPath || first.Origin.Line != 2 {
		t.Fatalf("rule 0 origin = %#v, want %s:2", first.Origin, routingPath)
	}
	if first.Origin.Text != "pname(curl) && dport(443) -> proxy" {
		t.Fatalf("rule 0 text = %q", first.Origin.Text)
	}
	// The must_ prefix is normalized into the outbound and its parameter, but
	// the text as written is kept for the API's expression.
	second := conf.Routing.Rules[1]
	if second.Outbound.Name != "direct" {
		t.Fatalf("rule 1 outbound = %q, want direct", second.Outbound.Name)
	}
	if second.Origin.File != routingPath || second.Origin.Line != 3 {
		t.Fatalf("rule 1 origin = %#v, want %s:3", second.Origin, routingPath)
	}
	if second.Origin.Text != "mac('00:e0:4c:2c:1a:b5') -> must_direct" {
		t.Fatalf("rule 1 text = %q", second.Origin.Text)
	}
	if conf.Routing.FallbackOrigin.File != routingPath || conf.Routing.FallbackOrigin.Line != 4 {
		t.Fatalf("routing fallback origin = %#v", conf.Routing.FallbackOrigin)
	}
	if conf.Routing.FallbackOrigin.Text != "fallback: direct" {
		t.Fatalf("routing fallback text = %q", conf.Routing.FallbackOrigin.Text)
	}

	if len(conf.Dns.Routing.Request.Rules) != 1 {
		t.Fatalf("dns request rules = %d", len(conf.Dns.Routing.Request.Rules))
	}
	dnsRule := conf.Dns.Routing.Request.Rules[0]
	if dnsRule.Origin.File != dnsPath || dnsRule.Origin.Line != 7 {
		t.Fatalf("dns rule origin = %#v, want %s:7", dnsRule.Origin, dnsPath)
	}
	if dnsRule.Origin.Text != "qname(suffix: example.com) -> alidns" {
		t.Fatalf("dns rule text = %q", dnsRule.Origin.Text)
	}
	if conf.Dns.Routing.Request.FallbackOrigin.File != dnsPath || conf.Dns.Routing.Request.FallbackOrigin.Line != 8 {
		t.Fatalf("dns request fallback origin = %#v", conf.Dns.Routing.Request.FallbackOrigin)
	}
	if conf.Dns.Routing.Response.FallbackOrigin.File != dnsPath || conf.Dns.Routing.Response.FallbackOrigin.Line != 11 {
		t.Fatalf("dns response fallback origin = %#v", conf.Dns.Routing.Response.FallbackOrigin)
	}
}

// TestUnwrittenFallbackHasNoOrigin keeps the two states apart: a fallback the
// configuration does not write has no location, which the API reports as a
// null source, not as a made-up line.
func TestUnwrittenFallbackHasNoOrigin(t *testing.T) {
	sections, err := config_parser.Parse("global{} routing{}")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	conf, err := New(sections)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if conf.Routing.FallbackOrigin.File != "" || conf.Routing.FallbackOrigin.Line != 0 {
		t.Fatalf("unwritten fallback origin = %#v", conf.Routing.FallbackOrigin)
	}
}
