/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package control

import (
	"github.com/daeuniverse/dae/config"
	"github.com/daeuniverse/dae/pkg/config_parser"
)

// DnsRuleSnapshot is one rendered DNS routing rule.
type DnsRuleSnapshot struct {
	Expression string
	Action     string
	Upstream   string
	// Origin is where the rule was written; empty for a fallback the
	// configuration does not write.
	Origin config_parser.Origin
}

// DnsRulesSnapshot is the complete DNS request/response rule dictionary.
type DnsRulesSnapshot struct {
	GenerationID     string
	Request          []DnsRuleSnapshot
	RequestFallback  DnsRuleSnapshot
	Response         []DnsRuleSnapshot
	ResponseFallback DnsRuleSnapshot
}

// MaximumDnsRuleDictionarySize bounds the advertised DNS rule capacity.
const MaximumDnsRuleDictionarySize = 1 << 16

func dnsRulesFromConfig(routing *config.DnsRouting, generation string) DnsRulesSnapshot {
	snapshot := DnsRulesSnapshot{GenerationID: generation}
	if routing == nil {
		return snapshot
	}
	snapshot.Request = renderDnsRuleList("request", routing.Request.Rules)
	snapshot.Response = renderDnsRuleList("response", routing.Response.Rules)
	snapshot.RequestFallback = renderDnsFallback("request", routing.Request.Fallback, routing.Request.FallbackOrigin)
	snapshot.ResponseFallback = renderDnsFallback("response", routing.Response.Fallback, routing.Response.FallbackOrigin)
	return snapshot
}

func renderDnsRuleList(kind string, rules []*config_parser.RoutingRule) []DnsRuleSnapshot {
	result := make([]DnsRuleSnapshot, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		action, upstream := dnsRuleAction(kind, rule.Outbound.Name)
		result = append(result, DnsRuleSnapshot{
			Expression: configuredRuleExpression(rule),
			Action:     action,
			Upstream:   upstream,
			Origin:     rule.Origin,
		})
	}
	return result
}

func renderDnsFallback(kind string, value config.FunctionOrString, origin config_parser.Origin) DnsRuleSnapshot {
	function, err := config.ParseFunctionOrString(value)
	if err != nil || function == nil {
		return DnsRuleSnapshot{Origin: origin}
	}
	expression := origin.Text
	if expression == "" {
		expression = function.MarshalString(false, false, true)
	}
	action, upstream := dnsRuleAction(kind, function.Name)
	return DnsRuleSnapshot{
		Expression: expression,
		Action:     action,
		Upstream:   upstream,
		Origin:     origin,
	}
}

// dnsRuleAction maps the target as written to the contract's action and
// upstream pair. In the configuration a request rule sends the query to a bare
// upstream name or to asis/reject, and a response rule requeries through a bare
// upstream name or accepts/rejects; the reserved words are actions, everything
// else names an upstream.
func dnsRuleAction(kind, name string) (action string, upstream string) {
	switch kind {
	case "request":
		switch name {
		case "asis", "reject":
			return name, ""
		}
		if name == "" {
			return "asis", ""
		}
		return "upstream", name
	default:
		switch name {
		case "accept", "reject":
			return name, ""
		}
		if name == "" {
			return "accept", ""
		}
		return "requery", name
	}
}

// SnapshotDnsRules projects this generation's DNS rule dictionary.
func (c *ControlPlane) SnapshotDnsRules() DnsRulesSnapshot {
	if c == nil {
		return DnsRulesSnapshot{}
	}
	return DnsRulesSnapshot{
		GenerationID:     c.dnsRules.GenerationID,
		Request:          append([]DnsRuleSnapshot(nil), c.dnsRules.Request...),
		Response:         append([]DnsRuleSnapshot(nil), c.dnsRules.Response...),
		RequestFallback:  c.dnsRules.RequestFallback,
		ResponseFallback: c.dnsRules.ResponseFallback,
	}
}
