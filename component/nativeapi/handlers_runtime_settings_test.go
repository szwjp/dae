/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package nativeapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeFlowProvider struct{}

func (fakeFlowProvider) ListFlows(ctx context.Context, filter FlowFilter) (FlowList, error) {
	return FlowList{}, nil
}

func (fakeFlowProvider) GetFlow(ctx context.Context, flowID string) (*FlowDetail, error) {
	return nil, nil
}

type fakeRuntimeSettingsProvider struct {
	settings RuntimeSettings
	patch    RuntimeSettingsPatch
	err      error
	fields   []string
	policy   string
}

func (p *fakeRuntimeSettingsProvider) GetRuntimeSettings(ctx context.Context) (RuntimeSettings, error) {
	return p.settings, p.err
}

func (p *fakeRuntimeSettingsProvider) PatchRuntimeSettings(ctx context.Context, patch RuntimeSettingsPatch) (RuntimeSettings, error) {
	p.patch = patch
	return p.settings, p.err
}

func (p *fakeRuntimeSettingsProvider) RuntimeSettingFields() []string {
	return p.fields
}

func (p *fakeRuntimeSettingsProvider) FlowRecordingPolicy() string {
	return p.policy
}

func runtimeSettingsServer(t *testing.T, provider RuntimeSettingsProvider) *Server {
	t.Helper()
	server := newTestServer(t, Config{AllowAnonymousLoopback: true}, fakeRuntimeProvider{runtime: testRuntime()})
	if provider != nil {
		server.SetRuntimeSettingsProvider(provider)
	}
	return server
}

func runtimeSettingsRequest(server *Server, method, path, body, contentType string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "127.0.0.1:9527"
	req.RemoteAddr = "127.0.0.1:1234"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)
	return recorder
}

func TestRuntimeSettingsGetReportsProviderDocument(t *testing.T) {
	provider := &fakeRuntimeSettingsProvider{
		settings: RuntimeSettings{
			ObservedAt: "2026-01-01T00:00:00Z",
			Source:     "config",
			Log:        &RuntimeLogSettings{Level: "info"},
			Recording: &RuntimeRecordingStates{
				Flows:  &RecorderState{Allowed: true, Mode: "auto", Active: true},
				Logs:   &RecorderState{Allowed: true, Mode: "auto", Active: true},
				DNSLog: &RecorderState{Allowed: true, Mode: "auto", Active: true},
			},
		},
		fields: []string{RuntimeSettingLogLevel},
	}
	server := runtimeSettingsServer(t, provider)

	response := doRequest(server, http.MethodGet, "/api/v1/runtime/settings", "127.0.0.1:1234", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	body := decodeJSON(t, response)
	if body["source"] != "config" {
		t.Fatalf("source = %v", body["source"])
	}
	recording := body["recording"].(map[string]any)
	if recording["flows"].(map[string]any)["active"] != true {
		t.Fatalf("recording = %#v", recording)
	}
}

func TestRuntimeSettingsGetWithoutProviderAnswers404(t *testing.T) {
	server := runtimeSettingsServer(t, nil)
	response := doRequest(server, http.MethodGet, "/api/v1/runtime/settings", "127.0.0.1:1234", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	body := decodeJSON(t, response)
	if body["error"].(map[string]any)["code"] != "capability_not_supported" {
		t.Fatalf("error = %#v", body["error"])
	}
}

func TestRuntimeSettingsPatchForwardsParsedBody(t *testing.T) {
	provider := &fakeRuntimeSettingsProvider{
		settings: RuntimeSettings{ObservedAt: "2026-01-01T00:00:00Z", Source: "runtime"},
		fields:   []string{RuntimeSettingRecordFlows},
	}
	server := runtimeSettingsServer(t, provider)

	body := `{"record_flows":"off","log":{"level":"debug"}}`
	response := runtimeSettingsRequest(server, http.MethodPatch, "/api/v1/runtime/settings", body, "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if provider.patch.RecordFlows == nil || *provider.patch.RecordFlows != "off" {
		t.Fatalf("record_flows patch = %#v", provider.patch.RecordFlows)
	}
	if provider.patch.Log == nil || provider.patch.Log.Level == nil || *provider.patch.Log.Level != "debug" {
		t.Fatalf("log patch = %#v", provider.patch.Log)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", response.Header().Get("X-Content-Type-Options"))
	}
}

func TestRuntimeSettingsPatchRejectsBadRequests(t *testing.T) {
	provider := &fakeRuntimeSettingsProvider{fields: []string{RuntimeSettingLogLevel}}
	server := runtimeSettingsServer(t, provider)

	cases := []struct {
		name        string
		body        string
		contentType string
		status      int
	}{
		{"missing content type", `{"record_flows":"off"}`, "", http.StatusUnsupportedMediaType},
		{"malformed json", `{"record_flows":`, "application/json", http.StatusBadRequest},
		{"unknown field", `{"nope":true}`, "application/json", http.StatusBadRequest},
		{"trailing data", `{}{}`, "application/json", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := runtimeSettingsRequest(server, http.MethodPatch, "/api/v1/runtime/settings", tc.body, tc.contentType)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}

func TestRuntimeSettingsMethodNotAllowed(t *testing.T) {
	server := runtimeSettingsServer(t, &fakeRuntimeSettingsProvider{})
	response := doRequest(server, http.MethodPost, "/api/v1/runtime/settings", "127.0.0.1:1234", nil)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", response.Code)
	}
}

func TestCapabilitiesAdvertiseRuntimeSettings(t *testing.T) {
	provider := &fakeRuntimeSettingsProvider{
		fields: []string{RuntimeSettingLogLevel, RuntimeSettingRecordFlows, RuntimeSettingRecordLogs, RuntimeSettingRecordDNS},
		policy: "off",
	}
	server := runtimeSettingsServer(t, provider)
	server.SetFlowProvider(&fakeFlowProvider{})

	response := doRequest(server, http.MethodGet, "/api/v1/capabilities", "127.0.0.1:1234", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	resources := decodeJSON(t, response)["resources"].(map[string]any)
	runtimeSettings, ok := resources["runtime_settings"].(map[string]any)
	if !ok || runtimeSettings["available"] != true {
		t.Fatalf("runtime_settings capability = %#v", resources["runtime_settings"])
	}
	fields := runtimeSettings["fields"].([]any)
	if len(fields) != 4 || fields[0] != RuntimeSettingLogLevel {
		t.Fatalf("fields = %#v", fields)
	}
	flows := resources["flows"].(map[string]any)
	if flows["recording"] != "off" {
		t.Fatalf("flows.recording = %v, want off", flows["recording"])
	}
}
