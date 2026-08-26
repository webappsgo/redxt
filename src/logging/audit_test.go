package logging

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// sampleAuditEntry is the PART 11 example entry, minus the fields
// Audit fills in itself.
func sampleAuditEntry() AuditEntry {
	return AuditEntry{
		Event:    "admin.login",
		Category: CategoryAuthentication,
		Severity: SeverityInfo,
		Actor: AuditActor{
			Type:      ActorAdmin,
			ID:        "administrator",
			IP:        "192.168.1.100",
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
		},
		Target: &AuditTarget{Type: "session", ID: "sess_abc123"},
		Details: map[string]any{
			"mfa_used":   true,
			"mfa_method": "totp",
		},
		Result: ResultSuccess,
	}
}

func TestAuditEntryShape(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{NodeID: "node-1"})
	logger.Audit(sampleAuditEntry())

	lines := readLog(t, dir, "audit.log")
	if len(lines) != 1 {
		t.Fatalf("audit.log has %d lines, want 1", len(lines))
	}
	line := lines[0]

	keys := []string{`"id":`, `"time":`, `"event":`, `"category":`, `"severity":`, `"actor":`, `"target":`, `"details":`, `"result":`, `"node_id":`}
	previous := -1
	for _, key := range keys {
		idx := strings.Index(line, key)
		if idx < 0 {
			t.Fatalf("audit entry is missing %s: %s", key, line)
		}
		if idx <= previous {
			t.Errorf("audit key %s is out of order in %s", key, line)
		}
		previous = idx
	}

	var decoded AuditEntry
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("audit entry is not valid JSON: %v", err)
	}
	if len(decoded.ID) != ulidLen {
		t.Errorf("audit id = %q, want a %d character ULID", decoded.ID, ulidLen)
	}
	if _, err := ParseULIDTime(decoded.ID); err != nil {
		t.Errorf("audit id is not a ULID: %v", err)
	}
	if decoded.NodeID != "node-1" {
		t.Errorf("node_id = %q, want node-1", decoded.NodeID)
	}
	if time.Time(decoded.Time).IsZero() {
		t.Error("audit time was not filled in")
	}
	if decoded.Actor.ID != "administrator" || decoded.Target.ID != "sess_abc123" {
		t.Errorf("actor or target lost: %+v", decoded)
	}
}

func TestAuditTimeHasMilliseconds(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	entry := sampleAuditEntry()
	entry.Time = AuditTime(time.Date(2025, 1, 15, 10, 30, 0, 123000000, time.UTC))
	logger.Audit(entry)

	line := readLog(t, dir, "audit.log")[0]
	if !strings.Contains(line, `"time":"2025-01-15T10:30:00.123Z"`) {
		t.Errorf("audit time not rendered with milliseconds in UTC: %s", line)
	}
}

func TestAuditOptionalFieldsAreOmitted(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	entry := sampleAuditEntry()
	entry.Target = nil
	entry.Details = nil
	logger.Audit(entry)

	line := readLog(t, dir, "audit.log")[0]
	for _, key := range []string{`"target"`, `"details"`} {
		if strings.Contains(line, key) {
			t.Errorf("empty %s should be omitted: %s", key, line)
		}
	}
}

func TestAuditRedactsDetails(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	entry := sampleAuditEntry()
	entry.Details = map[string]any{
		"password":    "placeholder-password-value",
		"api_token":   "placeholder-token-value",
		"tsig_secret": "placeholder-tsig-value",
		"nested": map[string]any{
			"private_key": "placeholder-key-material",
			"attempts":    3,
		},
		"attempts": 3,
	}
	logger.Audit(entry)

	line := readLog(t, dir, "audit.log")[0]
	leaks := []string{
		"placeholder-password-value",
		"placeholder-token-value",
		"placeholder-tsig-value",
		"placeholder-key-material",
	}
	for _, leak := range leaks {
		if strings.Contains(line, leak) {
			t.Errorf("audit entry leaked %q: %s", leak, line)
		}
	}
	if !strings.Contains(line, `"attempts":3`) {
		t.Errorf("audit entry dropped a non-sensitive detail: %s", line)
	}
}

func TestAuditSanitizesFreeText(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	entry := sampleAuditEntry()
	entry.Actor.UserAgent = "evil\x1b[31m\n{\"event\":\"forged\"}"
	logger.Audit(entry)

	lines := readLog(t, dir, "audit.log")
	if len(lines) != 1 {
		t.Fatalf("crafted user agent forged %d lines, want 1", len(lines))
	}
	if strings.Contains(lines[0], "\x1b") {
		t.Errorf("audit entry kept an escape sequence: %s", lines[0])
	}
}

func TestAuditDisabledWritesNothing(t *testing.T) {
	cfg := testLogs()
	cfg.Audit.Enabled = false
	logger, dir := newTestLogger(t, cfg, Options{})

	logger.Audit(sampleAuditEntry())

	if lines := readLog(t, dir, "audit.log"); len(lines) != 0 {
		t.Errorf("disabled audit log wrote %d lines: %v", len(lines), lines)
	}
}

func TestAuditWriteFailureReachesErrorLog(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	if err := logger.audit.file.Close(); err != nil {
		t.Fatalf("close audit file: %v", err)
	}

	logger.Audit(sampleAuditEntry())

	errLines := readLog(t, dir, "error.log")
	if len(errLines) != 1 || !strings.Contains(errLines[0], "audit log write failed") {
		t.Errorf("audit write failure was not reported to error.log: %v", errLines)
	}
}

func TestAuditIDsSortInWriteOrder(t *testing.T) {
	logger, dir := newTestLogger(t, testLogs(), Options{})
	for i := 0; i < 50; i++ {
		logger.Audit(sampleAuditEntry())
	}

	lines := readLog(t, dir, "audit.log")
	if len(lines) != 50 {
		t.Fatalf("audit.log has %d lines, want 50", len(lines))
	}

	previous := ""
	for i, line := range lines {
		var decoded AuditEntry
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
		if decoded.ID <= previous {
			t.Fatalf("audit id %q at line %d does not sort after %q", decoded.ID, i, previous)
		}
		previous = decoded.ID
	}
}
