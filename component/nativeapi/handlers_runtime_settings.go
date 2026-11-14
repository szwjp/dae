/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package nativeapi

import (
	"net/http"
	"time"
)

func (s *Server) handleRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if s.runtimeSettings == nil {
		writeError(w, requestID, http.StatusNotFound, codeCapabilityNotSupported, "resource is not supported by this engine")
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		settings, err := s.runtimeSettings.GetRuntimeSettings(r.Context())
		if err != nil {
			s.writeProviderFailure(w, requestID, err)
			return
		}
		s.writeRuntimeSettings(w, settings)
	case http.MethodPatch:
		if !isJSONContentType(r.Header.Get("Content-Type")) {
			writeError(w, requestID, http.StatusUnsupportedMediaType, codeUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
		var patch RuntimeSettingsPatch
		if err := decodeLimitedJSON(r, s.cfg.MaxJSONBodyBytes, &patch); err != nil {
			if err == errLimitedJSONBodyTooLarge {
				writeError(w, requestID, http.StatusRequestEntityTooLarge, codeInvalidRequest, "request body is too large")
				return
			}
			writeError(w, requestID, http.StatusBadRequest, codeInvalidRequest, "invalid JSON body: "+err.Error())
			return
		}
		settings, err := s.runtimeSettings.PatchRuntimeSettings(r.Context(), patch)
		if err != nil {
			s.writeProviderFailure(w, requestID, err)
			return
		}
		s.writeRuntimeSettings(w, settings)
	default:
		writeMethodNotAllowed(w, requestID, "GET, HEAD, PATCH")
	}
}

func (s *Server) writeRuntimeSettings(w http.ResponseWriter, settings RuntimeSettings) {
	if settings.ObservedAt == "" {
		settings.ObservedAt = time.Now().UTC().Format(time.RFC3339)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, settings)
}
