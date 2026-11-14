/*
 * SPDX-License-Identifier: AGPL-3.0-only
 * Copyright (c) 2022-2026, daeuniverse Organization <dae@v2raya.org>
 */

package control

import (
	"fmt"
	"testing"
	"time"
)

func TestDnsLogCapacityResizeKeepsNewest(t *testing.T) {
	controller := &DnsController{dnsControllerStore: newDnsControllerStore()}
	store := controller.dnsControllerStore
	store.dnsLogRing = make([]dnsLogRecord, 5)
	store.dnsLogLen = 5
	for i := range 5 {
		store.dnsLogRing[i] = dnsLogRecord{id: fmt.Sprintf("dl_%d", i)}
	}

	controller.SetDnsLogCapacity(3)
	if got := controller.DnsLogCapacity(); got != 3 {
		t.Fatalf("capacity = %d, want 3", got)
	}
	for i, want := range []string{"dl_2", "dl_3", "dl_4"} {
		if got := store.dnsLogRing[(store.dnsLogStart+i)%len(store.dnsLogRing)].id; got != want {
			t.Fatalf("record %d = %q, want %q", i, got, want)
		}
	}

	controller.SetDnsLogCapacity(8)
	if got := controller.DnsLogCapacity(); got != 8 {
		t.Fatalf("capacity after grow = %d, want 8", got)
	}
	if store.dnsLogLen != 3 {
		t.Fatalf("length after grow = %d, want 3", store.dnsLogLen)
	}
}

func TestFlowHistoryLimitsResizeKeepsNewest(t *testing.T) {
	manager := &SessionManager{}
	now := time.Now()
	for i := range 5 {
		manager.appendFlowHistory(SessionFlowSnapshot{ID: fmt.Sprintf("f_%d", i)}, now.Add(-time.Duration(6-i)*time.Second))
	}

	manager.SetFlowHistoryLimits(3, time.Minute)
	capacity, retention := manager.FlowHistoryLimits()
	if capacity != 3 || retention != time.Minute {
		t.Fatalf("limits = %d %v", capacity, retention)
	}
	entries, _ := manager.FlowHistorySnapshots()
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	for i, want := range []string{"f_2", "f_3", "f_4"} {
		if entries[i].ID != want {
			t.Fatalf("entry %d = %q, want %q", i, entries[i].ID, want)
		}
	}

	manager.SetFlowHistoryLimits(0, time.Second)
	if old, _ := manager.FlowHistorySnapshots(); len(old) != 0 {
		t.Fatalf("entries after retention shrink = %d, want 0", len(old))
	}
	manager.appendFlowHistory(SessionFlowSnapshot{ID: "f_5"}, now)
	if entries, _ := manager.FlowHistorySnapshots(); len(entries) != 1 {
		t.Fatalf("entries after append = %d, want 1", len(entries))
	}
}
