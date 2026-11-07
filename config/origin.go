/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package config

import "github.com/daeuniverse/dae/pkg/config_parser"

// attachFallbackOrigins copies the fallback locations the merger stamped on the
// parsed items into the decoded configuration. The rule dictionary reports
// where a fallback was written, and the fallback values themselves are decoded
// into FunctionOrString, which has no room for an origin.
//
// A fallback may be written in any of the files merged into one section; the
// last declaration wins, the way the section decoder applies it.
func attachFallbackOrigins(conf *Config, sections map[string]*config_parser.Section) {
	if conf == nil {
		return
	}
	if routing, ok := sections["routing"]; ok && routing != nil {
		conf.Routing.FallbackOrigin = lastDeclarationOrigin(routing.Items, "fallback")
	}
	dnsSection, ok := sections["dns"]
	if !ok || dnsSection == nil {
		return
	}
	for _, routing := range childSections(dnsSection.Items, "routing") {
		if request := lastChildSection(routing.Items, "request"); request != nil {
			conf.Dns.Routing.Request.FallbackOrigin = lastDeclarationOrigin(request.Items, "fallback")
		}
		if response := lastChildSection(routing.Items, "response"); response != nil {
			conf.Dns.Routing.Response.FallbackOrigin = lastDeclarationOrigin(response.Items, "fallback")
		}
	}
}

func childSections(items []*config_parser.Item, name string) []*config_parser.Section {
	result := make([]*config_parser.Section, 0)
	for _, item := range items {
		section, ok := item.Value.(*config_parser.Section)
		if !ok || section.Name != name {
			continue
		}
		result = append(result, section)
	}
	return result
}

func lastChildSection(items []*config_parser.Item, name string) *config_parser.Section {
	sections := childSections(items, name)
	if len(sections) == 0 {
		return nil
	}
	return sections[len(sections)-1]
}

// lastDeclarationOrigin returns the origin of the last declaration with the
// given key in items. A declaration is a Param with a key, which is how
// `fallback: proxy` is parsed.
func lastDeclarationOrigin(items []*config_parser.Item, key string) config_parser.Origin {
	origin := config_parser.Origin{}
	for _, item := range items {
		param, ok := item.Value.(*config_parser.Param)
		if !ok || param.Key != key {
			continue
		}
		origin = item.Origin
	}
	return origin
}
