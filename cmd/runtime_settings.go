/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"context"
	"net/http"
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
	mu       sync.Mutex
	log      *logrus.Logger
	logs     *frameHub
	flows    string
	logsMode string
	dns      string
	source   string
}

func newRuntimeSettings(log *logrus.Logger, logs *frameHub) *runtimeSettings {
	settings := &runtimeSettings{
		log:      log,
		logs:     logs,
		flows:    recorderModeAuto,
		logsMode: recorderModeAuto,
		dns:      recorderModeAuto,
		source:   "config",
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
	if patch.Log != nil {
		if patch.Log.BufferedRecords != nil {
			return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("log.buffered_records")
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
	if patch.DNSLog != nil && patch.DNSLog.MaxRecords != nil {
		return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("dns_log.max_records")
	}
	if patch.Flows != nil {
		if patch.Flows.MaxFlows != nil {
			return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("flows.max_flows")
		}
		if patch.Flows.RetentionSeconds != nil {
			return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("flows.retention_seconds")
		}
	}
	if len(patch.Geodata) > 0 && string(patch.Geodata) != "null" {
		return nativeapi.RuntimeSettings{}, unsupportedRuntimeField("geodata")
	}

	changed := patch.RecordFlows != nil || patch.RecordLogs != nil || patch.RecordDNSLog != nil ||
		(patch.Log != nil && patch.Log.Level != nil) || len(patch.Geodata) > 0
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
		Log:        &nativeapi.RuntimeLogSettings{Level: apiLogLevelName(r.levelLocked())},
		Recording: &nativeapi.RuntimeRecordingStates{
			Flows:  &nativeapi.RecorderState{Allowed: true, Mode: r.flows, Active: control.FlowStatsEnabled()},
			Logs:   &nativeapi.RecorderState{Allowed: true, Mode: r.logsMode, Active: r.logs == nil || r.logs.Recording()},
			DNSLog: &nativeapi.RecorderState{Allowed: true, Mode: r.dns, Active: control.DnsLogEnabled()},
		},
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
