/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package nativeapi

import (
	"context"
	"encoding/json"
)

// RuntimeSettingField names one setting PATCH /runtime/settings accepts. A
// field outside this list is defined by the contract but answers
// 422 unsupported_value on this engine.
const (
	RuntimeSettingLogLevel           = "log.level"
	RuntimeSettingLogBufferedRecords = "log.buffered_records"
	RuntimeSettingDNSLogMaxRecords   = "dns_log.max_records"
	RuntimeSettingFlowsMaxFlows      = "flows.max_flows"
	RuntimeSettingFlowsRetention     = "flows.retention_seconds"
	RuntimeSettingRecordFlows        = "record_flows"
	RuntimeSettingRecordLogs         = "record_logs"
	RuntimeSettingRecordDNS          = "record_dns_log"
)

// RuntimeSettings is the GET /runtime/settings document. Every section is
// optional: a value appears when RuntimeSettingsProvider.RuntimeSettingFields
// lists it.
type RuntimeSettings struct {
	ObservedAt string                  `json:"observed_at"`
	Source     string                  `json:"source"`
	Log        *RuntimeLogSettings     `json:"log,omitempty"`
	DNSLog     *RuntimeDNSLogSettings  `json:"dns_log,omitempty"`
	Flows      *RuntimeFlowsSettings   `json:"flows,omitempty"`
	Recording  *RuntimeRecordingStates `json:"recording,omitempty"`
}

// RuntimeLogSettings is the log section. buffered_records is reserved for the
// ring-capacity step and stays absent until that field is advertised.
type RuntimeLogSettings struct {
	Level           string  `json:"level,omitempty"`
	BufferedRecords *uint64 `json:"buffered_records,omitempty"`
}

// RuntimeRecordingStates reports each recorder's allowed/mode/active state.
// RuntimeDNSLogSettings is the dns_log section.
type RuntimeDNSLogSettings struct {
	MaxRecords uint64 `json:"max_records"`
}

// RuntimeFlowsSettings is the flows section.
type RuntimeFlowsSettings struct {
	MaxFlows         uint64 `json:"max_flows,omitempty"`
	RetentionSeconds uint64 `json:"retention_seconds,omitempty"`
}

type RuntimeRecordingStates struct {
	Flows  *RecorderState `json:"flows,omitempty"`
	Logs   *RecorderState `json:"logs,omitempty"`
	DNSLog *RecorderState `json:"dns_log,omitempty"`
}

// RecorderState says whether a recorder is permitted, the mode last set by a
// PATCH (auto when none was set) and whether it captures right now.
type RecorderState struct {
	Allowed bool   `json:"allowed"`
	Mode    string `json:"mode"`
	Active  bool   `json:"active"`
}

// RuntimeSettingsPatch is the merge body PATCH accepts. Sections the schema
// defines but this engine does not implement are carried so the handler can
// answer 422 unsupported_value instead of 400 invalid_request.
type RuntimeSettingsPatch struct {
	RecordFlows  *string             `json:"record_flows,omitempty"`
	RecordLogs   *string             `json:"record_logs,omitempty"`
	RecordDNSLog *string             `json:"record_dns_log,omitempty"`
	Log          *RuntimeLogPatch    `json:"log,omitempty"`
	DNSLog       *RuntimeDNSLogPatch `json:"dns_log,omitempty"`
	Flows        *RuntimeFlowsPatch  `json:"flows,omitempty"`
	Geodata      json.RawMessage     `json:"geodata,omitempty"`
}

type RuntimeLogPatch struct {
	Level           *string `json:"level,omitempty"`
	BufferedRecords *uint64 `json:"buffered_records,omitempty"`
}

type RuntimeDNSLogPatch struct {
	MaxRecords *uint64 `json:"max_records,omitempty"`
}

type RuntimeFlowsPatch struct {
	MaxFlows         *uint64 `json:"max_flows,omitempty"`
	RetentionSeconds *uint64 `json:"retention_seconds,omitempty"`
}

// RuntimeSettingsProvider serves and changes settings that apply without a
// reload. A nil provider makes the resource unavailable in capabilities.
type RuntimeSettingsProvider interface {
	GetRuntimeSettings(ctx context.Context) (RuntimeSettings, error)
	PatchRuntimeSettings(ctx context.Context, patch RuntimeSettingsPatch) (RuntimeSettings, error)
	// RuntimeSettingFields lists the settings this engine accepts; any other
	// contract field answers 422 unsupported_value.
	RuntimeSettingFields() []string
	// FlowRecordingPolicy reports the engine's flow recording policy for
	// resources.flows.recording: off, on, auto or sampled.
	FlowRecordingPolicy() string
}
