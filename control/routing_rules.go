/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package control

import (
	"strconv"

	"github.com/daeuniverse/dae/component/routing"
	"github.com/daeuniverse/dae/config"
	"github.com/daeuniverse/dae/pkg/config_parser"
)

// RoutingRuleSnapshot is one rendered rule of a running routing generation.
type RoutingRuleSnapshot struct {
	Expression string
	Outbound   string
	Must       bool
	// Origin is where the rule was written; empty when the engine built it
	// without a source (an injected or programmatic rule).
	Origin config_parser.Origin
}

// RoutingRulesSnapshot is the complete rule dictionary of one generation.
type RoutingRulesSnapshot struct {
	GenerationID       string
	Rules              []RoutingRuleSnapshot
	FallbackOutbound   string
	FallbackExpression string
	FallbackOrigin     config_parser.Origin
}

// configuredRuleExpression is the rule's text as the operator wrote it when
// the parser kept the source, and the engine's rendering otherwise.
func configuredRuleExpression(rule *config_parser.RoutingRule) string {
	if rule == nil {
		return ""
	}
	if rule.Origin.Text != "" {
		return rule.Origin.Text
	}
	return rule.String(false, false, false)
}

// ruleOutbound renders the outbound without the must marker, which the
// contract reports in the separate must boolean. must_direct is normalized to
// the direct outbound plus that marker before the snapshot is taken, so both
// spellings land here as outbound "direct" with must true.
func ruleOutbound(rule *config_parser.RoutingRule) string {
	if rule == nil {
		return ""
	}
	outbound := rule.Outbound
	params := make([]*config_parser.Param, 0, len(outbound.Params))
	for _, param := range outbound.Params {
		if param != nil && param.Key == "" && param.Val == "must" {
			continue
		}
		params = append(params, param)
	}
	outbound.Params = params
	return outbound.MarshalString(false, true, true)
}

// ruleMust reports whether the rule was written as a must rule, either
// must_direct / must_<group> or <group>(must); the patch that normalizes the
// first form has already run by the time the snapshot is taken.
func ruleMust(rule *config_parser.RoutingRule) bool {
	if rule == nil {
		return false
	}
	for _, param := range rule.Outbound.Params {
		if param != nil && param.Key == "" && param.Val == "must" {
			return true
		}
	}
	return false
}

// snapshotConfiguredRules renders the rules exactly as the operator wrote them.
//
// It must be handed the parsed configuration, never the optimized program: the
// optimizers expand geosite/geoip data into hundreds of thousands of parameters
// and rewrite selector names, so exporting the evaluation order produced an
// 8.5 MB response describing rules nobody wrote. The configured form is both the
// truthful answer and orders of magnitude smaller.
func snapshotConfiguredRules(rules []*config_parser.RoutingRule, fallback config.FunctionOrString, fallbackOrigin config_parser.Origin) ([]RoutingRuleSnapshot, string, string) {
	snapshots := make([]RoutingRuleSnapshot, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		snapshots = append(snapshots, RoutingRuleSnapshot{
			Expression: configuredRuleExpression(rule),
			Outbound:   ruleOutbound(rule),
			Must:       ruleMust(rule),
			Origin:     rule.Origin,
		})
	}
	parsed, err := config.ParseFunctionOrString(fallback)
	if err != nil || parsed == nil {
		return snapshots, "", ""
	}
	expression := fallbackOrigin.Text
	if expression == "" {
		expression = "fallback: " + parsed.MarshalString(false, false, true)
	}
	return snapshots, parsed.MarshalString(false, true, true), expression
}

func routingGenerationID(epoch routing.PolicyEpoch) string {
	return strconv.FormatUint(uint64(epoch), 10)
}

// SnapshotRoutingRules projects this generation's routing dictionary.
func (c *ControlPlane) SnapshotRoutingRules() RoutingRulesSnapshot {
	if c == nil {
		return RoutingRulesSnapshot{}
	}
	return RoutingRulesSnapshot{
		GenerationID:       c.routingGeneration,
		Rules:              append([]RoutingRuleSnapshot(nil), c.routingRules...),
		FallbackOutbound:   c.routingFallback,
		FallbackExpression: c.routingFallbackExpression,
		FallbackOrigin:     c.routingFallbackOrigin,
	}
}

// RoutingRuleExpressionByIndex returns the display expression for a rule index
// of this generation, for flow evidence.
func (c *ControlPlane) RoutingRuleExpressionByIndex(index int) (string, bool) {
	if c == nil || index < 0 || index >= len(c.routingRules) {
		return "", false
	}
	return c.routingRules[index].Expression, true
}
