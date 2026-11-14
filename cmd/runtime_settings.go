/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/daeuniverse/dae/component/nativeapi"
	"github.com/daeuniverse/dae/control"
	"github.com/sirupsen/logrus"
)

const (
	recorderModeAuto = "auto"
	recorderModeOn   = "on"
	recorderModeOff  = "off"
)

var runtimeSettingFields = []string{
	nativeapi.RuntimeSettingLogLevel,
	nativeapi.RuntimeSettingLogBufferedRecords,
	nativeapi.RuntimeSettingDNSLogMaxRecords,
	nativeapi.RuntimeSettingFlowsMaxFlows,
	nativeapi.RuntimeSettingFlowsRetention,
	nativeapi.RuntimeSettingRecordFlows,
	nativeapi.RuntimeSettingRecordLogs,
	nativeapi.RuntimeSettingRecordDNS,
}

// runtimeSettings holds the settings a caller may change without a reload.
// Modes start at auto, which records like the previous always-on behaviour;
// off stops the recorder until the next PATCH or configuration activation.
// Nothing is written to the configuration: a reload resets every mode through
// Reset, and the reload itself re-applies the configured log level.
type runtimeSettings struct {
	mu         sync.Mutex
	log        *logrus.Logger
	logs       *frameHub
	supervisor *runtimeSupervisor
	flows      string
	logsMode   string
	dns        string
	source     string
}

func newRuntimeSettings(log *logrus.Logger, logs *frameHub, supervisor *runtimeSupervisor) *runtimeSettings {
	settings := &runtimeSettings{
		log:        log,
		logs:       logs,
		supervisor: supervisor,
		flows:      recorderModeAuto,
		logsMode:   recorderModeAuto,
		dns:        recorderModeAuto,
		source:     "config",
	}
	if logs != nil {
		logs.SetRecording(true)
	}
	return settings
}

// Reset returns every setting to the value the configuration just activated.
// The reload path has already applied the configured log level and the record
// switches, so this only drops the overrides and resumes the log ring.
func (r *runtimeSettings) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flows = recorderModeAuto
	r.logsMode = recorderModeAuto
	r.dns = recorderModeAuto
	r.source = "config"
	if r.logs != nil {
		r.logs.SetCapacity(nativeapi.LogMaxBufferedRecords)
	}
	r.setDNSLogCapacity(nativeapi.DnsLogMaxRecords)
	if r.supervisor != nil {
		if active := r.supervisor.activeSnapshot(); active != nil && active.controlPlane != nil {
			active.controlPlane.SetFlowHistoryLimits(control.FlowHistoryCapacity, control.FlowHistoryRetention)
		}
	}
	control.ConfigureFlowStats(true)
	control.ConfigureDnsLog(true)
	if r.logs != nil {
		r.logs.SetRecording(true)
	}
}

func (r *runtimeSettings) RuntimeSettingFields() []string {
	return append([]string(nil), runtimeSettingFields...)
}

func (r *runtimeSettings) FlowRecordingPolicy() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.flows == recorderModeOff {
		return "off"
	}
	return "on"
}

func (r *runtimeSettings) GetRuntimeSettings(ctx context.Context) (nativeapi.RuntimeSettings, error) {
	if err := ctx.Err(); err != nil {
		return nativeapi.RuntimeSettings{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked(), nil
}

func (r *runtimeSettings) PatchRuntimeSettings(ctx context.Context, patch nativeapi.RuntimeSettingsPatch) (nativeapi.RuntimeSettings, error) {
	if err := ctx.Err(); err != nil {
		return nativeapi.RuntimeSettings{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate the whole patch before changing anything: a rejected field must
	// leave every other value alone.
	flowsMode, logsMode, dnsMode := r.flows, r.logsMode, r.dns
	if patch.RecordFlows != nil {
		if !validRecorderMode(*patch.RecordFlows) {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue("record_flows must be auto, on or off")
		}
		flowsMode = *patch.RecordFlows
	}
	if patch.RecordLogs != nil {
		if !validRecorderMode(*patch.RecordLogs) {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue("record_logs must be auto, on or off")
		}
		logsMode = *patch.RecordLogs
	}
	if patch.RecordDNSLog != nil {
		if !validRecorderMode(*patch.RecordDNSLog) {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue("record_dns_log must be auto, on or off")
		}
		dnsMode = *patch.RecordDNSLog
	}

	var level logrus.Level
	var logCapacity *int
	if patch.Log != nil {
		if patch.Log.BufferedRecords != nil {
			if *patch.Log.BufferedRecords < 1 || *patch.Log.BufferedRecords > nativeapi.LogMaxBufferedRecords {
				return nativeapi.RuntimeSettings{}, invalidRuntimeValue(
					"log.buffered_records must be between 1 and " + strconv.Itoa(nativeapi.LogMaxBufferedRecords))
			}
			capacity := int(*patch.Log.BufferedRecords)
			logCapacity = &capacity
		}
		if patch.Log.Level != nil {
			if !validLogLevel(*patch.Log.Level) {
				return nativeapi.RuntimeSettings{}, &nativeapi.APIError{
					Status: http.StatusUnprocessableEntity, Code: "unsupported_value",
					Message: "log.level must be one of the levels resources.logs.levels reports",
				}
			}
			parsed, err := logrus.ParseLevel(*patch.Log.Level)
			if err != nil {
				return nativeapi.RuntimeSettings{}, invalidRuntimeValue("log.level is not a log level")
			}
			level = parsed
		}
	}
	var dnsCapacity *int
	if patch.DNSLog != nil && patch.DNSLog.MaxRecords != nil {
		if *patch.DNSLog.MaxRecords < 1 || *patch.DNSLog.MaxRecords > nativeapi.DnsLogMaxRecords {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue(
				"dns_log.max_records must be between 1 and " + strconv.Itoa(nativeapi.DnsLogMaxRecords))
		}
		capacity := int(*patch.DNSLog.MaxRecords)
		dnsCapacity = &capacity
	}
	var flowsCapacity *int
	if patch.Flows != nil && patch.Flows.MaxFlows != nil {
		if *patch.Flows.MaxFlows < 1 || *patch.Flows.MaxFlows > control.FlowHistoryCapacity {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue(
				"flows.max_flows must be between 1 and " + strconv.Itoa(control.FlowHistoryCapacity))
		}
		capacity := int(*patch.Flows.MaxFlows)
		flowsCapacity = &capacity
	}
	var flowsRetention *time.Duration
	if patch.Flows != nil && patch.Flows.RetentionSeconds != nil {
		maxRetention := int64(control.FlowHistoryRetention / time.Second)
		if *patch.Flows.RetentionSeconds < 1 || int64(*patch.Flows.RetentionSeconds) > maxRetention {
			return nativeapi.RuntimeSettings{}, invalidRuntimeValue(
				"flows.retention_seconds must be between 1 and " + strconv.FormatInt(maxRetention, 10))
		}
		retention := time.Duration(*patch.Flows.RetentionSeconds) * time.Second
		flowsRetention = &retention
	}
	if len(patch.Geodata) > 0 && string(patch.Geodata) != "null" {
		return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("geodata")
	}

	if (dnsCapacity != nil || flowsCapacity != nil || flowsRetention != nil) && !r.hasActiveControlPlane() {
		return nativeapi.RuntimeSettings{}, &nativeapi.APIError{
			Status: http.StatusServiceUnavailable, Code: "temporarily_unavailable",
			Message: "no active generation can take the ring limits",
		}
	}
	changed := patch.RecordFlows != nil || patch.RecordLogs != nil || patch.RecordDNSLog != nil ||
		(patch.Log != nil && (patch.Log.Level != nil || patch.Log.BufferedRecords != nil)) ||
		dnsCapacity != nil || flowsCapacity != nil || flowsRetention != nil || len(patch.Geodata) > 0
	if !changed {
		return r.snapshotLocked(), nil
	}

	// Apply: the values were validated above, so a failure here cannot leave a
	// half-applied patch.
	if patch.Log != nil && patch.Log.Level != nil {
		if r.log != nil {
			r.log.SetLevel(level)
		}
		logrus.SetLevel(level)
	}
	if logCapacity != nil && r.logs != nil {
		r.logs.SetCapacity(*logCapacity)
	}
	if dnsCapacity != nil {
		r.setDNSLogCapacity(*dnsCapacity)
	}
	if flowsCapacity != nil || flowsRetention != nil {
		active := r.supervisor.activeSnapshot()
		capacity, retention := active.controlPlane.FlowHistoryLimits()
		if flowsCapacity != nil {
			capacity = *flowsCapacity
		}
		if flowsRetention != nil {
			retention = *flowsRetention
		}
		active.controlPlane.SetFlowHistoryLimits(capacity, retention)
	}
	r.flows = flowsMode
	r.logsMode = logsMode
	r.dns = dnsMode
	control.ConfigureFlowStats(flowsMode != recorderModeOff)
	control.ConfigureDnsLog(dnsMode != recorderModeOff)
	if r.logs != nil {
		r.logs.SetRecording(logsMode != recorderModeOff)
	}
	r.source = "runtime"
	return r.snapshotLocked(), nil
}

func (r *runtimeSettings) snapshotLocked() nativeapi.RuntimeSettings {
	settings := nativeapi.RuntimeSettings{
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Source:     r.source,
		Log:        r.logSettingsLocked(),
		DNSLog:     &nativeapi.RuntimeDNSLogSettings{MaxRecords: uint64(r.dnsLogCapacity())},
		Flows:      r.flowsSettingsLocked(),
		Recording: &nativeapi.RuntimeRecordingStates{
			Flows:  &nativeapi.RecorderState{Allowed: true, Mode: r.flows, Active: control.FlowStatsEnabled()},
			Logs:   &nativeapi.RecorderState{Allowed: true, Mode: r.logsMode, Active: r.logs == nil || r.logs.Recording()},
			DNSLog: &nativeapi.RecorderState{Allowed: true, Mode: r.dns, Active: control.DnsLogEnabled()},
		},
	}
	return settings
}

func (r *runtimeSettings) flowsSettingsLocked() *nativeapi.RuntimeFlowsSettings {
	capacity, retention := control.FlowHistoryCapacity, control.FlowHistoryRetention
	if r.supervisor != nil {
		if active := r.supervisor.activeSnapshot(); active != nil && active.controlPlane != nil {
			capacity, retention = active.controlPlane.FlowHistoryLimits()
		}
	}
	return &nativeapi.RuntimeFlowsSettings{
		MaxFlows:         uint64(capacity),
		RetentionSeconds: uint64(retention / time.Second),
	}
}

func (r *runtimeSettings) dnsLogCapacity() int {
	if r.supervisor != nil {
		if active := r.supervisor.activeSnapshot(); active != nil && active.controlPlane != nil {
			if capacity := active.controlPlane.DnsLogCapacity(); capacity > 0 {
				return capacity
			}
		}
	}
	return nativeapi.DnsLogMaxRecords
}

func (r *runtimeSettings) hasActiveControlPlane() bool {
	if r.supervisor == nil {
		return false
	}
	active := r.supervisor.activeSnapshot()
	return active != nil && active.controlPlane != nil
}

func (r *runtimeSettings) setDNSLogCapacity(capacity int) bool {
	if r.supervisor == nil {
		return false
	}
	active := r.supervisor.activeSnapshot()
	if active == nil || active.controlPlane == nil {
		return false
	}
	active.controlPlane.SetDnsLogCapacity(capacity)
	return true
}

func (r *runtimeSettings) logSettingsLocked() *nativeapi.RuntimeLogSettings {
	settings := &nativeapi.RuntimeLogSettings{Level: apiLogLevelName(r.levelLocked())}
	if r.logs != nil {
		capacity := uint64(r.logs.Capacity())
		settings.BufferedRecords = &capacity
	}
	return settings
}

func (r *runtimeSettings) levelLocked() logrus.Level {
	if r.log != nil {
		return r.log.GetLevel()
	}
	return logrus.GetLevel()
}

func apiLogLevelName(level logrus.Level) string {
	switch level {
	case logrus.TraceLevel:
		return "trace"
	case logrus.DebugLevel:
		return "debug"
	case logrus.InfoLevel:
		return "info"
	case logrus.WarnLevel:
		return "warn"
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return "error"
	default:
		return "info"
	}
}

func validRecorderMode(mode string) bool {
	return mode == recorderModeAuto || mode == recorderModeOn || mode == recorderModeOff
}

func validLogLevel(level string) bool {
	switch level {
	case "trace", "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

func unsupportedRuntimeField(field string) error {
	return &nativeapi.APIError{
		Status: http.StatusUnprocessableEntity, Code: "unsupported_value",
		Message: field + " is not runtime-adjustable on this engine",
	}
}

func invalidRuntimeValue(message string) error {
	return &nativeapi.APIError{
		Status: http.StatusBadRequest, Code: "invalid_request", Message: message,
	}
}
