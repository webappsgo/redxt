package logging

import (
	"encoding/json"
	"time"

	"github.com/webappsgo/redxt/src/security"
)

// Audit severity levels from the AI.md PART 11 "Severity Levels"
// table.
const (
	// SeverityInfo marks a successful normal operation.
	SeverityInfo = "info"
	// SeverityWarn marks a failed attempt or a recoverable issue.
	SeverityWarn = "warn"
	// SeverityError marks a failure that needs attention.
	SeverityError = "error"
	// SeverityCritical marks a security incident or a server failure.
	SeverityCritical = "critical"
)

// Audit event categories from the PART 11 audit "events" configuration
// block.
const (
	// CategoryAuthentication covers login and logout events.
	CategoryAuthentication = "authentication"
	// CategoryConfiguration covers configuration changes.
	CategoryConfiguration = "configuration"
	// CategorySecurity covers security events.
	CategorySecurity = "security"
	// CategoryTokens covers token creation and revocation.
	CategoryTokens = "tokens"
	// CategoryUsers covers user management.
	CategoryUsers = "users"
	// CategoryBackup covers backup and restore.
	CategoryBackup = "backup"
	// CategoryServer covers server start, stop, and maintenance.
	CategoryServer = "server"
	// CategoryCluster covers cluster membership events.
	CategoryCluster = "cluster"
	// CategoryTokenUsage covers individual token uses, which are high
	// volume and disabled by default.
	CategoryTokenUsage = "token_usage"
)

// Audit results from the PART 11 required-field table.
const (
	// ResultSuccess marks an action that completed.
	ResultSuccess = "success"
	// ResultFailure marks an action that did not complete.
	ResultFailure = "failure"
)

// Audit actor types.
const (
	// ActorAdmin is a server administrator.
	ActorAdmin = "admin"
	// ActorUser is an end user.
	ActorUser = "user"
	// ActorToken is an API token acting on its own.
	ActorToken = "token"
	// ActorSystem is the server itself, for scheduled and internal
	// actions.
	ActorSystem = "system"
)

// AuditTime is an audit timestamp. It marshals as ISO 8601 in UTC with
// exactly three fractional digits, which is the millisecond precision
// PART 11 requires and which the default time.Time encoding does not
// guarantee.
type AuditTime time.Time

// MarshalJSON encodes the timestamp in UTC with milliseconds.
func (t AuditTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(t).UTC().Format(millisTimeLayout))
}

// UnmarshalJSON decodes a timestamp written by MarshalJSON.
func (t *AuditTime) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := time.Parse(millisTimeLayout, raw)
	if err != nil {
		return err
	}
	*t = AuditTime(parsed)
	return nil
}

// IsZero reports whether the timestamp is unset.
func (t AuditTime) IsZero() bool {
	return time.Time(t).IsZero()
}

// AuditActor identifies who performed an audited action.
type AuditActor struct {
	// Type is the actor kind: admin, user, token, or system.
	Type string `json:"type"`
	// ID identifies the actor within its kind.
	ID string `json:"id"`
	// IP is the source address of the action.
	IP string `json:"ip"`
	// UserAgent is the client user agent, when one is available.
	UserAgent string `json:"user_agent,omitempty"`
}

// AuditTarget identifies what an audited action acted upon.
type AuditTarget struct {
	// Type is the target kind, such as session, user, or zone.
	Type string `json:"type"`
	// ID identifies the target within its kind.
	ID string `json:"id"`
}

// AuditEntry is one audit record. The field order matches the PART 11
// example exactly, because the JSON encoding follows the declaration
// order of the struct fields.
type AuditEntry struct {
	// ID is the ULID of this entry. Audit fills it when it is empty.
	ID string `json:"id"`
	// Time is when the event happened. Audit fills it when it is
	// unset.
	Time AuditTime `json:"time"`
	// Event is the event type, such as admin.login.
	Event string `json:"event"`
	// Category is the event category, one of the Category constants.
	Category string `json:"category"`
	// Severity is one of the Severity constants.
	Severity string `json:"severity"`
	// Actor is who performed the action.
	Actor AuditActor `json:"actor"`
	// Target is what was acted upon, omitted when there is none.
	Target *AuditTarget `json:"target,omitempty"`
	// Details carries event-specific fields. Every value is passed
	// through security.RedactMap before it is written.
	Details map[string]any `json:"details,omitempty"`
	// Result is ResultSuccess or ResultFailure.
	Result string `json:"result"`
	// NodeID is the cluster node that produced the entry.
	NodeID string `json:"node_id,omitempty"`
}

// Audit writes one JSON Lines audit record.
//
// The entry is completed before it is written: an empty ID becomes a
// fresh ULID, an unset time becomes the current UTC time, an empty
// node ID becomes the logger's configured node, and Details is passed
// through security.RedactMap so no password, token, key, or other
// sensitive field can reach the file. Every free-text field is
// sanitized, so a crafted user agent cannot forge a second record.
//
// The audit log is the authoritative record of who did what, so a
// write failure is never dropped silently: it is reported to
// error.log. Audit output is JSON only; the configured format is not
// consulted because PART 11 allows no other encoding for this log.
// Nothing is written when the audit log is disabled in the
// configuration.
func (l *Logger) Audit(e AuditEntry) {
	if !l.auditEnabled || l.audit == nil || l.audit.file == nil {
		return
	}

	if e.ID == "" {
		e.ID = NewULID()
	}
	if e.Time.IsZero() {
		e.Time = AuditTime(time.Now().UTC())
	}
	if e.NodeID == "" {
		e.NodeID = l.nodeID
	}
	e.Event = sanitize(e.Event)
	e.Category = sanitize(e.Category)
	e.Severity = sanitize(e.Severity)
	e.Result = sanitize(e.Result)
	e.NodeID = sanitize(e.NodeID)
	e.Actor = sanitizeActor(e.Actor)
	if e.Target != nil {
		e.Target = &AuditTarget{
			Type: sanitize(e.Target.Type),
			ID:   sanitize(e.Target.ID),
		}
	}
	e.Details = security.RedactMap(e.Details)

	encoded, err := json.Marshal(e)
	if err != nil {
		l.Errorf("audit log encode failed for event %q: %v", e.Event, err)
		return
	}
	if err := l.audit.write(newPlainLine(string(encoded))); err != nil {
		l.reportWriteFailure("audit", err)
	}
}

// sanitizeActor strips control characters and escape sequences from
// every actor field.
func sanitizeActor(a AuditActor) AuditActor {
	return AuditActor{
		Type:      sanitize(a.Type),
		ID:        sanitize(a.ID),
		IP:        sanitize(a.IP),
		UserAgent: sanitize(a.UserAgent),
	}
}
