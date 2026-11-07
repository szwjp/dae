/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package control

import (
	"net/netip"

	"github.com/daeuniverse/dae/common/consts"
	"github.com/daeuniverse/dae/component/outbound"
	"github.com/daeuniverse/dae/component/routing"
	"github.com/daeuniverse/dae/pkg/config_parser"
)

type controlPlaneGenerationState struct {
	outbounds                 []*outbound.DialerGroup
	routingRules              []RoutingRuleSnapshot
	routingFallback           string
	routingFallbackExpression string
	routingFallbackOrigin     config_parser.Origin
	routingGeneration         string
	dnsRules                  DnsRulesSnapshot
	referencedOutbounds       map[string]struct{}
	dialMode                  consts.DialMode
	policyIdentity            routing.PolicyIdentity
	routingMatcher            *RoutingMatcher
	bootstrapResolvers        []netip.AddrPort
}
