/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package cmd

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/daeuniverse/dae/component/nativeapi"
	"github.com/daeuniverse/dae/control"
	"github.com/sirupsen/logrus"
)

func newTestRuntimeSettings(t *testing.T) (*runtimeSettings, *logrus.Logger, *frameHub) {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	control.ConfigureFlowStats(true)
	control.ConfigureDnsLog(true)
	hub := newFrameHub("instance-test", "logs", nativeapi.LogMaxBufferedRecords, time.Minute, 1)
	settings := newRuntimeSettings(log, hub, nil)
	t.Cleanup(func() {
		control.ConfigureFlowStats(true)
		control.ConfigureDnsLog(true)
		logrus.SetLevel(logrus.InfoLevel)
	})
	return settings, log, hub
}

func TestRuntimeSettingsDefaults(t *testing.T) {
	settings, log, hub := newTestRuntimeSettings(t)

	got, err := settings.GetRuntimeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "config" {
		t.Fatalf("source = %q, want config", got.Source)
	}
	if got.Log == nil || got.Log.Level != "info" {
		t.Fatalf("log = %#v", got.Log)
	}
	if got.Log.BufferedRecords == nil || *got.Log.BufferedRecords != nativeapi.LogMaxBufferedRecords {
		t.Fatalf("buffered_records = %#v", got.Log.BufferedRecords)
	}
	if got.DNSLog == nil || got.DNSLog.MaxRecords != nativeapi.DnsLogMaxRecords {
		t.Fatalf("dns_log = %#v", got.DNSLog)
	}
	if got.Flows == nil || got.Flows.MaxFlows != control.FlowHistoryCapacity ||
		got.Flows.RetentionSeconds != uint64(control.FlowHistoryRetention/time.Second) {
		t.Fatalf("flows = %#v", got.Flows)
	}
	if got.Recording == nil || got.Recording.Flows == nil || got.Recording.Logs == nil || got.Recording.DNSLog == nil {
		t.Fatalf("recording = %#v", got.Recording)
	}
	for name, state := range map[string]*nativeapi.RecorderState{
		"flows": got.Recording.Flows, "logs": got.Recording.Logs, "dns_log": got.Recording.DNSLog,
	} {
		if !state.Allowed || state.Mode != "auto" || !state.Active {
			t.Fatalf("%s state = %#v", name, state)
		}
	}
	if !control.FlowStatsEnabled() || !control.DnsLogEnabled() || !hub.Recording() {
		t.Fatal("recorders are not running at startup")
	}
	if fields := settings.RuntimeSettingFields(); len(fields) != 8 {
		t.Fatalf("fields = %#v", fields)
	}
	if log.GetLevel() != logrus.InfoLevel {
		t.Fatalf("logger level = %v", log.GetLevel())
	}
}

func TestRuntimeSettingsPatchRecorders(t *testing.T) {
	settings, _, hub := newTestRuntimeSettings(t)
	ctx := context.Background()

	off := "off"
	updated, err := settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{
		RecordFlows:  &off,
		RecordLogs:   &off,
		RecordDNSLog: &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source != "runtime" {
		t.Fatalf("source = %q, want runtime", updated.Source)
	}
	for name, state := range map[string]*nativeapi.RecorderState{
		"flows": updated.Recording.Flows, "logs": updated.Recording.Logs, "dns_log": updated.Recording.DNSLog,
	} {
		if state.Mode != "off" || state.Active {
			t.Fatalf("%s state = %#v", name, state)
		}
	}
	if control.FlowStatsEnabled() || control.DnsLogEnabled() || hub.Recording() {
		t.Fatal("recorders kept running after off")
	}
	if policy := settings.FlowRecordingPolicy(); policy != "off" {
		t.Fatalf("policy = %q, want off", policy)
	}

	on := "on"
	updated, err = settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{RecordFlows: &on})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Recording.Flows.Mode != "on" || !updated.Recording.Flows.Active {
		t.Fatalf("flows state = %#v", updated.Recording.Flows)
	}
	if !control.FlowStatsEnabled() {
		t.Fatal("flow stats stayed off")
	}
}

func TestRuntimeSettingsPatchLogLevel(t *testing.T) {
	settings, log, _ := newTestRuntimeSettings(t)
	level := "debug"

	updated, err := settings.PatchRuntimeSettings(context.Background(), nativeapi.RuntimeSettingsPatch{
		Log: &nativeapi.RuntimeLogPatch{Level: &level},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Log.Level != "debug" {
		t.Fatalf("level = %q", updated.Log.Level)
	}
	if log.GetLevel() != logrus.DebugLevel {
		t.Fatalf("logger level = %v", log.GetLevel())
	}
	if updated.Source != "runtime" {
		t.Fatalf("source = %q", updated.Source)
	}
}

func TestRuntimeSettingsPatchLogCapacity(t *testing.T) {
	settings, _, hub := newTestRuntimeSettings(t)
	ctx := context.Background()

	capacity := uint64(64)
	updated, err := settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{
		Log: &nativeapi.RuntimeLogPatch{BufferedRecords: &capacity},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Log.BufferedRecords == nil || *updated.Log.BufferedRecords != 64 {
		t.Fatalf("buffered_records = %#v", updated.Log.BufferedRecords)
	}
	if got := hub.Capacity(); got != 64 {
		t.Fatalf("hub capacity = %d, want 64", got)
	}
	if updated.Source != "runtime" {
		t.Fatalf("source = %q", updated.Source)
	}

	tooLarge := uint64(nativeapi.LogMaxBufferedRecords + 1)
	if _, err := settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{
		Log: &nativeapi.RuntimeLogPatch{BufferedRecords: &tooLarge},
	}); err == nil {
		t.Fatal("oversized capacity was accepted")
	}
	zero := uint64(0)
	if _, err := settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{
		Log: &nativeapi.RuntimeLogPatch{BufferedRecords: &zero},
	}); err == nil {
		t.Fatal("zero capacity was accepted")
	}

	settings.Reset()
	if got := hub.Capacity(); got != nativeapi.LogMaxBufferedRecords {
		t.Fatalf("hub capacity after reset = %d", got)
	}
}

func TestRuntimeSettingsDNSCapacityNeedsActiveGeneration(t *testing.T) {
	settings, _, _ := newTestRuntimeSettings(t)
	capacity := uint64(64)
	_, err := settings.PatchRuntimeSettings(context.Background(), nativeapi.RuntimeSettingsPatch{
		DNSLog: &nativeapi.RuntimeDNSLogPatch{MaxRecords: &capacity},
	})
	apiErr, ok := err.(*nativeapi.APIError)
	if !ok || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503", err)
	}
}

func TestRuntimeSettingsFlowLimitsValidation(t *testing.T) {
	settings, _, _ := newTestRuntimeSettings(t)
	ctx := context.Background()
	tooMany := uint64(control.FlowHistoryCapacity + 1)
	zero := uint64(0)
	tooLong := uint64(control.FlowHistoryRetention/time.Second) + 1

	for name, patch := range map[string]nativeapi.RuntimeSettingsPatch{
		"too many flows":    {Flows: &nativeapi.RuntimeFlowsPatch{MaxFlows: &tooMany}},
		"zero flows":        {Flows: &nativeapi.RuntimeFlowsPatch{MaxFlows: &zero}},
		"retention too big": {Flows: &nativeapi.RuntimeFlowsPatch{RetentionSeconds: &tooLong}},
		"retention zero":    {Flows: &nativeapi.RuntimeFlowsPatch{RetentionSeconds: &zero}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := settings.PatchRuntimeSettings(ctx, patch)
			apiErr, ok := err.(*nativeapi.APIError)
			if !ok || apiErr.Status != http.StatusBadRequest {
				t.Fatalf("err = %v, want 400", err)
			}
		})
	}
}

func TestRuntimeSettingsFlowLimitsNeedActiveGeneration(t *testing.T) {
	settings, _, _ := newTestRuntimeSettings(t)
	capacity := uint64(64)
	_, err := settings.PatchRuntimeSettings(context.Background(), nativeapi.RuntimeSettingsPatch{
		Flows: &nativeapi.RuntimeFlowsPatch{MaxFlows: &capacity},
	})
	apiErr, ok := err.(*nativeapi.APIError)
	if !ok || apiErr.Status != http.StatusServiceUnavailable {
		t.Fatalf("err = %v, want 503", err)
	}
}

func TestRuntimeSettingsRejectsUnsupportedFields(t *testing.T) {
	settings, _, _ := newTestRuntimeSettings(t)
	ctx := context.Background()
	snippet := "x"
	level := "verbose"

	cases := []struct {
		name  string
		patch nativeapi.RuntimeSettingsPatch
		code  string
	}{
		{"geodata", nativeapi.RuntimeSettingsPatch{Geodata: []byte(`{"auto_update":true}`)}, "unsupported_value"},
		{"bad level", nativeapi.RuntimeSettingsPatch{Log: &nativeapi.RuntimeLogPatch{Level: &level}}, "unsupported_value"},
		{"bad mode", nativeapi.RuntimeSettingsPatch{RecordFlows: &snippet}, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := settings.PatchRuntimeSettings(ctx, tc.patch)
			apiErr, ok := err.(*nativeapi.APIError)
			if !ok {
				t.Fatalf("err = %v, want APIError", err)
			}
			if apiErr.Code != tc.code {
				t.Fatalf("code = %q, want %q", apiErr.Code, tc.code)
			}
			if apiErr.Status != http.StatusUnprocessableEntity && apiErr.Status != http.StatusBadRequest {
				t.Fatalf("status = %d", apiErr.Status)
			}
		})
	}

	// A rejected patch changes nothing.
	got, err := settings.GetRuntimeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "config" || got.Recording.Flows.Mode != "auto" || !control.FlowStatsEnabled() {
		t.Fatalf("settings changed after rejected patches: %#v", got)
	}
}

func TestRuntimeSettingsResetDropsOverrides(t *testing.T) {
	settings, _, hub := newTestRuntimeSettings(t)
	off := "off"
	if _, err := settings.PatchRuntimeSettings(context.Background(), nativeapi.RuntimeSettingsPatch{RecordFlows: &off, RecordLogs: &off}); err != nil {
		t.Fatal(err)
	}

	settings.Reset()

	got, err := settings.GetRuntimeSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "config" {
		t.Fatalf("source = %q, want config", got.Source)
	}
	if got.Recording.Flows.Mode != "auto" || !got.Recording.Flows.Active {
		t.Fatalf("flows = %#v", got.Recording.Flows)
	}
	if !control.FlowStatsEnabled() || !hub.Recording() {
		t.Fatal("recorders did not resume after reset")
	}
}

func TestRuntimeSettingsEmptyPatchKeepsValues(t *testing.T) {
	settings, _, _ := newTestRuntimeSettings(t)
	ctx := context.Background()
	before, err := settings.GetRuntimeSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := settings.PatchRuntimeSettings(ctx, nativeapi.RuntimeSettingsPatch{})
	if err != nil {
		t.Fatal(err)
	}
	if after.Source != before.Source || after.Recording.Flows.Mode != before.Recording.Flows.Mode {
		t.Fatalf("empty patch changed settings: %#v -> %#v", before, after)
	}
}

func TestFrameHubRecordingGate(t *testing.T) {
	hub := newFrameHub("instance", "logs", 4, time.Minute, 1)
	if !hub.Recording() {
		t.Fatal("hub starts not recording")
	}
	hub.SetRecording(false)
	hub.publish("log", "dropped")
	if got := hubLength(hub); got != 0 {
		t.Fatalf("hub retained %d frames while off", got)
	}
	hub.SetRecording(true)
	hub.publish("log", "kept")
	if got := hubLength(hub); got != 1 {
		t.Fatalf("hub retained %d frames after resume", got)
	}
}

func TestFrameHubResizeKeepsNewest(t *testing.T) {
	hub := newFrameHub("instance", "logs", 8, time.Minute, 1)
	for _, value := range []string{"a", "b", "c"} {
		hub.publish("log", value)
	}
	hub.SetCapacity(2)
	if got := hub.Capacity(); got != 2 {
		t.Fatalf("capacity = %d, want 2", got)
	}
	held := hub.heldFrames()
	if len(held) != 2 || held[0].data != "b" || held[1].data != "c" {
		t.Fatalf("held = %#v", held)
	}
	hub.SetCapacity(4)
	if got := hub.Capacity(); got != 4 {
		t.Fatalf("capacity after grow = %d, want 4", got)
	}
	if got := hubLength(hub); got != 2 {
		t.Fatalf("length after grow = %d, want 2", got)
	}
}

func (h *frameHub) heldFrames() []storedFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	frames := make([]storedFrame, 0, h.length)
	for i := 0; i < h.length; i++ {
		frames = append(frames, h.frames[(h.start+i)%len(h.frames)])
	}
	return frames
}

func hubLength(h *frameHub) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.length
}

func TestAPILogLevelName(t *testing.T) {
	cases := map[logrus.Level]string{
		logrus.TraceLevel: "trace",
		logrus.DebugLevel: "debug",
		logrus.InfoLevel:  "info",
		logrus.WarnLevel:  "warn",
		logrus.ErrorLevel: "error",
		logrus.FatalLevel: "error",
		logrus.PanicLevel: "error",
	}
	for level, want := range cases {
		if got := apiLogLevelName(level); got != want {
			t.Fatalf("%v -> %q, want %q", level, got, want)
		}
	}
}
