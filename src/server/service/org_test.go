package service

import (
	"context"
	"errors"
	"testing"

	"github.com/webappsgo/redxt/src/user"
)

// newZone inserts a zone directly, bypassing the (not yet implemented)
// zone service, so GrantZone can be exercised against a zone that really
// belongs to a given organization.
func newZone(t *testing.T, svc *Service, orgID int64, name string) int64 {
	t.Helper()

	res, err := svc.store.DB().ExecContext(context.Background(),
		`INSERT INTO zones (org_id, name) VALUES (?, ?)`, orgID, name)
	if err != nil {
		t.Fatalf("insert zone %s: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("zone LastInsertId: %v", err)
	}
	return id
}

// TestGrantZoneRejectsCrossOrgZone covers AUDIT.AI.md Pass 1 item 1: an
// organization must not be able to grant, and thereby reference, a zone
// that belongs to a different organization.
func TestGrantZoneRejectsCrossOrgZone(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ownerA := newUser(t, svc, "ownera")
	orgA := newOrg(t, svc, ownerA.ID, "org-a")
	zoneA := newZone(t, svc, orgA.ID, "a.example.test")

	ownerB := newUser(t, svc, "ownerb")
	orgB := newOrg(t, svc, ownerB.ID, "org-b")
	editorB := newUser(t, svc, "editorb")
	addMember(t, svc, orgB, ownerB.ID, editorB, user.RoleEditor)

	// orgB's owner tries to grant a member authority over orgA's zone.
	err := svc.GrantZone(ctx, orgB.ID, ownerB.ID, editorB.ID, zoneA, "edit")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("GrantZone cross-org = %v, want ErrForbidden", err)
	}

	granted, err := svc.store.ZoneGranted(ctx, orgB.ID, editorB.ID, zoneA)
	if err != nil {
		t.Fatalf("ZoneGranted: %v", err)
	}
	if granted {
		t.Fatal("cross-org GrantZone recorded a grant, want no grant stored")
	}
}

// TestGrantZoneAllowsSameOrgZone confirms the ownership check does not
// block the case it must keep working: granting a zone the organization
// actually owns.
func TestGrantZoneAllowsSameOrgZone(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	ownerA := newUser(t, svc, "ownera")
	orgA := newOrg(t, svc, ownerA.ID, "org-a")
	zoneA := newZone(t, svc, orgA.ID, "a.example.test")

	editorA := newUser(t, svc, "editora")
	addMember(t, svc, orgA, ownerA.ID, editorA, user.RoleEditor)

	if err := svc.GrantZone(ctx, orgA.ID, ownerA.ID, editorA.ID, zoneA, "edit"); err != nil {
		t.Fatalf("GrantZone same-org: %v", err)
	}

	granted, err := svc.store.ZoneGranted(ctx, orgA.ID, editorA.ID, zoneA)
	if err != nil {
		t.Fatalf("ZoneGranted: %v", err)
	}
	if !granted {
		t.Fatal("same-org GrantZone did not record a grant")
	}
}
