/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package config_parser

import "testing"

func TestParseRecordsRuleOrigins(t *testing.T) {
	sections, err := Parse(`routing {
	pname(curl) && dport(443) -> proxy
	mac('00:e0:4c:2c:1a:b5') -> must_direct
	fallback: direct
}
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(sections) != 1 || sections[0].Name != "routing" {
		t.Fatalf("sections = %#v", sections)
	}
	items := sections[0].Items
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}

	rule, ok := items[0].Value.(*RoutingRule)
	if !ok {
		t.Fatalf("item 0 = %T", items[0].Value)
	}
	want := Origin{
		Line:   2,
		Column: 2,
		Text:   "pname(curl) && dport(443) -> proxy",
	}
	if rule.Origin.Line != want.Line || rule.Origin.Column != want.Column || rule.Origin.Text != want.Text {
		t.Fatalf("rule 0 origin = %#v, want %#v", rule.Origin, want)
	}

	second, ok := items[1].Value.(*RoutingRule)
	if !ok {
		t.Fatalf("item 1 = %T", items[1].Value)
	}
	if second.Origin.Line != 3 || second.Origin.Text != "mac('00:e0:4c:2c:1a:b5') -> must_direct" {
		t.Fatalf("rule 1 origin = %#v", second.Origin)
	}
	if second.Origin.Text == "" {
		t.Fatal("the text as written must be retained")
	}

	fallback, ok := items[2].Value.(*Param)
	if !ok || fallback.Key != "fallback" {
		t.Fatalf("item 2 = %#v", items[2].Value)
	}
	if items[2].Origin.Line != 4 || items[2].Origin.Text != "fallback: direct" {
		t.Fatalf("fallback origin = %#v", items[2].Origin)
	}
}

func TestParseRecordsMultilineRuleOrigin(t *testing.T) {
	sections, err := Parse(`routing {
	domain(geosite:cn) &&
	dip(geoip:private) -> direct
}
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rule := sections[0].Items[0].Value.(*RoutingRule)
	if rule.Origin.Line != 2 {
		t.Fatalf("line = %d, want 2", rule.Origin.Line)
	}
	if rule.Origin.Text != "domain(geosite:cn) &&\n\tdip(geoip:private) -> direct" {
		t.Fatalf("text = %q", rule.Origin.Text)
	}
}
