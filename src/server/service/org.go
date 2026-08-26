package service

import (
	"context"
	"strings"
	"time"

	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/user"
)

// CreationMode returns the effective PART 35 organization creation mode.
// As with registration, an unparsable value closes creation rather than
// opening it.
func (s *Service) CreationMode() user.CreationMode {
	mode, err := user.ParseCreationMode(s.orgs().Creation.Mode)
	if err != nil {
		return user.CreationDisabled
	}
	return mode
}

// createPersonalOrg makes the organization every account gets.
//
// It is not subject to server.orgs.creation.mode: that policy governs
// how many shared organizations a user may start, while the personal org
// is infrastructure. Without one, a user in a deployment with creation
// disabled would have nowhere to attach a zone, and PART 35 attaches
// every zone to an organization.
func (s *Service) createPersonalOrg(ctx context.Context, account model.User) (model.Org, error) {
	org, err := s.store.CreateOrg(ctx, model.Org{
		Slug:       account.Username,
		Name:       account.DisplayName,
		Visibility: s.defaultOrgVisibility(),
		Personal:   true,
		OwnerID:    account.ID,
		Status:     model.StatusActive,
	})
	if err != nil {
		return model.Org{}, mapStoreErr(err)
	}

	s.audit(ctx, org.ID, EventOrgCreatedPersonal, model.ActorSystem, 0, account.ID, nil)
	return org, nil
}

// joinInvitedOrg adds a newly registered user to the organization that
// invited them, at the role the invitation named.
func (s *Service) joinInvitedOrg(ctx context.Context, invite model.Invite, account model.User) error {
	role, err := user.ParseRole(invite.Role)
	if err != nil {
		role = s.defaultMemberRole()
	}
	if err = s.store.AddMember(ctx, invite.OrgID, account.ID, string(role)); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, invite.OrgID, store.EventMemberJoined, model.ActorUser,
		account.ID, account.ID, map[string]any{"role": string(role)})
	return nil
}

// defaultOrgVisibility returns the configured organization visibility,
// defaulting to private for an unrecognized value.
func (s *Service) defaultOrgVisibility() string {
	if s.orgs().Profile.DefaultVisibility == model.VisibilityPublic {
		return model.VisibilityPublic
	}
	return model.VisibilityPrivate
}

// defaultMemberRole returns the role a new member joins at, falling back
// to the least privileged role when the configured value is unknown.
func (s *Service) defaultMemberRole() user.Role {
	role, err := user.ParseRole(s.orgs().Members.DefaultRole)
	if err != nil || role == user.RoleOwner {
		return user.RoleViewer
	}
	return role
}

// CreateOrgInput is one shared organization being started.
type CreateOrgInput struct {
	Slug        string
	Name        string
	Description string
	Website     string
	Location    string
	// InviteCode is the plaintext org-creation invite, required in
	// invite mode.
	InviteCode string
	// ByAdmin marks creation performed by a Server Admin, the only path
	// allowed in admin_only mode.
	ByAdmin bool
}

// CreateOrg starts a shared organization owned by the given user.
func (s *Service) CreateOrg(ctx context.Context, ownerID int64, in CreateOrgInput) (model.Org, error) {
	if !s.orgs().Enabled {
		return model.Org{}, ErrDisabled
	}

	// Which policy this request must satisfy is decided by how it
	// arrived, not by the mode alone. Open mode accepts invites without
	// requiring them, so testing the invite policy first would demand a
	// code from every member of an open server.
	mode := s.CreationMode()
	switch {
	case in.ByAdmin:
		if !mode.AdminCreateAllowed() {
			return model.Org{}, ErrDisabled
		}
	case in.InviteCode != "":
		if !mode.InviteAllowed() {
			return model.Org{}, ErrDisabled
		}
		if _, err := s.redeemInvite(ctx, in.InviteCode); err != nil {
			return model.Org{}, err
		}
	case !mode.SelfServiceAllowed():
		return model.Org{}, ErrForbidden
	}

	slug, err := user.ValidateName(in.Slug)
	if err != nil {
		return model.Org{}, validationError(err.Error())
	}

	if max := s.orgs().Creation.MaxPerUser; max > 0 && !in.ByAdmin {
		owned, countErr := s.store.CountOwnedOrgs(ctx, ownerID)
		if countErr != nil {
			return model.Org{}, countErr
		}
		if owned >= max {
			return model.Org{}, ErrQuotaExceeded
		}
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = slug
	}

	org, err := s.store.CreateOrg(ctx, model.Org{
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Website:     strings.TrimSpace(in.Website),
		Location:    strings.TrimSpace(in.Location),
		Visibility:  s.defaultOrgVisibility(),
		Personal:    false,
		OwnerID:     ownerID,
		Status:      model.StatusActive,
	})
	if err != nil {
		return model.Org{}, mapStoreErr(err)
	}

	s.audit(ctx, org.ID, store.EventOrgCreated, model.ActorUser, ownerID, 0, nil)
	return org, nil
}

// OrgAccess is a caller's resolved standing inside one organization.
type OrgAccess struct {
	Org  model.Org
	Role user.Role
}

// Can reports whether the caller's role carries a permission.
func (a OrgAccess) Can(p user.Permission) bool {
	return a.Role.Can(p)
}

// Access resolves a caller's membership in an organization.
//
// An organization the caller does not belong to is reported as not
// found, never as forbidden. Distinguishing the two would let a caller
// walk the identifier space and learn which organizations exist, which
// is the IDOR the PART 35 isolation rule exists to prevent.
func (s *Service) Access(ctx context.Context, orgID, userID int64) (OrgAccess, error) {
	org, err := s.store.OrgByID(ctx, orgID)
	if err != nil {
		return OrgAccess{}, mapStoreErr(err)
	}

	member, err := s.store.Membership(ctx, orgID, userID)
	if err != nil {
		return OrgAccess{}, ErrNotFound
	}
	role, err := user.ParseRole(member.Role)
	if err != nil {
		return OrgAccess{}, ErrNotFound
	}
	return OrgAccess{Org: org, Role: role}, nil
}

// AccessBySlug resolves a caller's membership from an organization's
// slug, which is how PART 13 addresses an organization in a URL.
//
// An unknown slug and a slug the caller has no membership in answer the
// same way, for the same reason Access does not distinguish them.
func (s *Service) AccessBySlug(ctx context.Context, slug string, userID int64) (OrgAccess, error) {
	org, err := s.store.OrgBySlug(ctx, slug)
	if err != nil {
		return OrgAccess{}, ErrNotFound
	}

	member, err := s.store.Membership(ctx, org.ID, userID)
	if err != nil {
		return OrgAccess{}, ErrNotFound
	}
	role, err := user.ParseRole(member.Role)
	if err != nil {
		return OrgAccess{}, ErrNotFound
	}
	return OrgAccess{Org: org, Role: role}, nil
}

// require resolves access and checks one permission in a single step,
// which is the form every org-scoped handler uses.
func (s *Service) require(ctx context.Context, orgID, userID int64, p user.Permission) (OrgAccess, error) {
	access, err := s.Access(ctx, orgID, userID)
	if err != nil {
		return OrgAccess{}, err
	}
	if !access.Can(p) {
		return OrgAccess{}, ErrForbidden
	}
	return access, nil
}

// Require exposes the permission check to the handler layer.
func (s *Service) Require(ctx context.Context, orgID, userID int64, p user.Permission) (OrgAccess, error) {
	return s.require(ctx, orgID, userID, p)
}

// OrgsForUser lists the organizations a caller belongs to.
func (s *Service) OrgsForUser(ctx context.Context, userID int64) ([]model.Org, error) {
	orgs, err := s.store.OrgsForUser(ctx, userID)
	return orgs, mapStoreErr(err)
}

// UpdateOrg writes the editable organization profile.
func (s *Service) UpdateOrg(ctx context.Context, orgID, actorID int64, in CreateOrgInput) (model.Org, error) {
	access, err := s.require(ctx, orgID, actorID, user.PermOrgSettings)
	if err != nil {
		return model.Org{}, err
	}

	org := access.Org
	if name := strings.TrimSpace(in.Name); name != "" {
		org.Name = name
	}
	org.Description = strings.TrimSpace(in.Description)
	org.Website = strings.TrimSpace(in.Website)
	org.Location = strings.TrimSpace(in.Location)

	if err = s.store.UpdateOrg(ctx, org); err != nil {
		return model.Org{}, mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventOrgUpdated, model.ActorUser, actorID, 0, nil)
	return org, nil
}

// SetOrgVisibility changes whether the organization has a public
// profile.
func (s *Service) SetOrgVisibility(ctx context.Context, orgID, actorID int64, visibility string) error {
	if visibility != model.VisibilityPublic && visibility != model.VisibilityPrivate {
		return validationError("visibility must be public or private")
	}

	access, err := s.require(ctx, orgID, actorID, user.PermOrgSettings)
	if err != nil {
		return err
	}

	org := access.Org
	org.Visibility = visibility
	if err = s.store.UpdateOrg(ctx, org); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventOrgUpdated, model.ActorUser, actorID, 0,
		map[string]any{"visibility": visibility})
	return nil
}

// DeleteOrg removes a shared organization.
//
// A personal organization is refused: it is the only home a solo user's
// zones have, and deleting it would orphan them. Deleting the account
// removes it instead, through the cascade.
func (s *Service) DeleteOrg(ctx context.Context, orgID, actorID int64) error {
	access, err := s.require(ctx, orgID, actorID, user.PermOrgDelete)
	if err != nil {
		return err
	}
	if access.Org.Personal {
		return ErrForbidden
	}
	if err = s.store.DeleteOrg(ctx, orgID); err != nil {
		return mapStoreErr(err)
	}
	return nil
}

// TransferOrg hands ownership to another member.
func (s *Service) TransferOrg(ctx context.Context, orgID, actorID, newOwnerID int64) error {
	access, err := s.Access(ctx, orgID, actorID)
	if err != nil {
		return err
	}
	// Only the current owner may transfer, whatever the permission table
	// says about org settings: ownership is not a settings change.
	if access.Role != user.RoleOwner || access.Org.OwnerID != actorID {
		return ErrForbidden
	}
	if access.Org.Personal {
		return ErrForbidden
	}
	if newOwnerID == actorID {
		return validationError("the new owner must be a different member")
	}
	if err = s.store.TransferOrg(ctx, orgID, actorID, newOwnerID); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventOrgTransferred, model.ActorUser, actorID,
		newOwnerID, nil)
	return nil
}

// Members lists an organization's members for a caller who may view it.
func (s *Service) Members(ctx context.Context, orgID, actorID int64) ([]model.Member, error) {
	if _, err := s.require(ctx, orgID, actorID, user.PermRead); err != nil {
		return nil, err
	}
	members, err := s.store.ListMembers(ctx, orgID)
	return members, mapStoreErr(err)
}

// SetMemberRole changes a member's role.
//
// The caller must both hold the member-management permission and outrank
// the roles on each side of the change, so an Admin cannot promote
// anyone to Owner or demote the Owner, and no member can escalate
// themselves.
func (s *Service) SetMemberRole(ctx context.Context, orgID, actorID, targetID int64, role string) error {
	access, err := s.require(ctx, orgID, actorID, user.PermMembersManage)
	if err != nil {
		return err
	}
	if access.Org.Personal {
		return ErrForbidden
	}

	next, err := user.ParseRole(role)
	if err != nil {
		return validationError("unknown organization role")
	}

	current, err := s.store.Membership(ctx, orgID, targetID)
	if err != nil {
		return ErrNotFound
	}
	existing, err := user.ParseRole(current.Role)
	if err != nil {
		return ErrNotFound
	}

	if !access.Role.CanManageMember(existing) || !access.Role.CanGrantRole(next) {
		return ErrForbidden
	}
	if err = s.store.SetMemberRole(ctx, orgID, targetID, string(next)); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventMemberRoleSet, model.ActorUser, actorID,
		targetID, map[string]any{"from": string(existing), "to": string(next)})
	return nil
}

// RemoveMember removes a member and, with them, their org-scoped
// credentials and zone grants.
func (s *Service) RemoveMember(ctx context.Context, orgID, actorID, targetID int64) error {
	access, err := s.Access(ctx, orgID, actorID)
	if err != nil {
		return err
	}
	if access.Org.Personal {
		return ErrForbidden
	}
	if access.Org.OwnerID == targetID {
		return ErrForbidden
	}

	// A member may always remove themselves; removing anyone else needs
	// the management permission and a higher rank.
	if targetID != actorID {
		if !access.Can(user.PermMembersManage) {
			return ErrForbidden
		}
		current, memberErr := s.store.Membership(ctx, orgID, targetID)
		if memberErr != nil {
			return ErrNotFound
		}
		existing, roleErr := user.ParseRole(current.Role)
		if roleErr != nil || !access.Role.CanManageMember(existing) {
			return ErrForbidden
		}
	}

	if err = s.store.RemoveMember(ctx, orgID, targetID); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventMemberRemoved, model.ActorUser, actorID,
		targetID, nil)
	return nil
}

// InviteInput is one invitation being issued.
type InviteInput struct {
	Email string
	Role  string
	// MaxUses is how many times the code may be redeemed. Zero means
	// unlimited.
	MaxUses int
}

// InviteResult carries a freshly issued invitation and its plaintext
// code, which exists only in the returned value and the message sent to
// the invitee.
type InviteResult struct {
	Invite model.Invite
	Code   string
}

// InviteMember issues an invitation into an organization.
func (s *Service) InviteMember(ctx context.Context, orgID, actorID int64, in InviteInput) (InviteResult, error) {
	access, err := s.require(ctx, orgID, actorID, user.PermMembersManage)
	if err != nil {
		return InviteResult{}, err
	}
	if access.Org.Personal {
		return InviteResult{}, ErrForbidden
	}
	if !s.orgs().Members.AllowInvites && access.Role != user.RoleOwner {
		return InviteResult{}, ErrForbidden
	}

	role := s.defaultMemberRole()
	if in.Role != "" {
		parsed, roleErr := user.ParseRole(in.Role)
		if roleErr != nil {
			return InviteResult{}, validationError("unknown organization role")
		}
		// The invitation cannot confer more authority than the inviter
		// holds, which is the same cap applied to token issue.
		role = user.CapRole(access.Role, parsed)
	}

	email := ""
	if trimmed := strings.TrimSpace(in.Email); trimmed != "" {
		normalized, mailErr := user.ValidateEmail(trimmed)
		if mailErr != nil {
			return InviteResult{}, validationError(mailErr.Error())
		}
		email = normalized
	}

	maxUses := in.MaxUses
	if maxUses < 0 {
		maxUses = 1
	}

	code, err := security.RandomString(security.RandomLength)
	if err != nil {
		return InviteResult{}, err
	}

	days := s.users().Registration.InviteExpirationDays
	if days <= 0 {
		days = 7
	}

	invite, err := s.store.CreateInvite(ctx, model.Invite{
		OrgID:     orgID,
		Email:     email,
		Role:      string(role),
		CodeHash:  security.HashToken(code),
		MaxUses:   maxUses,
		InvitedBy: actorID,
		ExpiresAt: s.now().Add(time.Duration(days) * 24 * time.Hour),
	})
	if err != nil {
		return InviteResult{}, mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventMemberInvited, model.ActorUser, actorID, 0,
		map[string]any{"role": string(role), "email": security.MaskEmail(email)})

	return InviteResult{Invite: invite, Code: code}, nil
}

// AcceptInvite adds an existing user to the organization an invitation
// names.
func (s *Service) AcceptInvite(ctx context.Context, userID int64, code string) (model.Org, error) {
	invite, err := s.redeemInvite(ctx, code)
	if err != nil {
		return model.Org{}, err
	}
	if invite.OrgID == 0 {
		return model.Org{}, ErrForbidden
	}

	role, err := user.ParseRole(invite.Role)
	if err != nil {
		role = s.defaultMemberRole()
	}
	if err = s.store.AddMember(ctx, invite.OrgID, userID, string(role)); err != nil {
		if mapStoreErr(err) == ErrConflict {
			return s.orgByID(ctx, invite.OrgID)
		}
		return model.Org{}, mapStoreErr(err)
	}

	s.audit(ctx, invite.OrgID, store.EventMemberJoined, model.ActorUser, userID,
		userID, map[string]any{"role": string(role)})

	return s.orgByID(ctx, invite.OrgID)
}

// ListInvites returns an organization's outstanding invitations.
func (s *Service) ListInvites(ctx context.Context, orgID, actorID int64) ([]model.Invite, error) {
	if _, err := s.require(ctx, orgID, actorID, user.PermMembersManage); err != nil {
		return nil, err
	}
	invites, err := s.store.ListOrgInvites(ctx, orgID)
	return invites, mapStoreErr(err)
}

// RevokeInvite withdraws an invitation.
func (s *Service) RevokeInvite(ctx context.Context, orgID, actorID, inviteID int64) error {
	if _, err := s.require(ctx, orgID, actorID, user.PermMembersManage); err != nil {
		return err
	}

	// The invitation is looked up through the organization's own list, so
	// an id belonging to another organization is reported as not found
	// rather than deleted.
	invites, err := s.store.ListOrgInvites(ctx, orgID)
	if err != nil {
		return mapStoreErr(err)
	}
	for _, invite := range invites {
		if invite.ID == inviteID {
			if err = s.store.DeleteInvite(ctx, inviteID); err != nil {
				return mapStoreErr(err)
			}
			s.audit(ctx, orgID, store.EventInviteRevoked, model.ActorUser,
				actorID, 0, nil)
			return nil
		}
	}
	return ErrNotFound
}

// GrantZone gives a member authority over one zone, which is what the
// Editor role is scoped by.
func (s *Service) GrantZone(ctx context.Context, orgID, actorID, targetID, zoneID int64, permission string) error {
	if _, err := s.require(ctx, orgID, actorID, user.PermMembersManage); err != nil {
		return err
	}
	if _, err := s.store.Membership(ctx, orgID, targetID); err != nil {
		return ErrNotFound
	}
	if permission == "" {
		permission = "edit"
	}
	if err := s.store.GrantZone(ctx, orgID, targetID, zoneID, permission); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventZoneGranted, model.ActorUser, actorID,
		targetID, map[string]any{"zone_id": zoneID, "permission": permission})
	return nil
}

// RevokeZone removes a member's authority over one zone.
func (s *Service) RevokeZone(ctx context.Context, orgID, actorID, targetID, zoneID int64) error {
	if _, err := s.require(ctx, orgID, actorID, user.PermMembersManage); err != nil {
		return err
	}
	if err := s.store.RevokeZone(ctx, orgID, targetID, zoneID); err != nil {
		return mapStoreErr(err)
	}

	s.audit(ctx, orgID, store.EventZoneRevoked, model.ActorUser, actorID,
		targetID, map[string]any{"zone_id": zoneID})
	return nil
}

// CanEditZone reports whether a caller may change records in one zone.
//
// An Owner or Admin may edit any zone the organization holds. An Editor
// may edit only the zones explicitly granted to them, which is what
// makes the Editor role narrower than Admin rather than merely differently
// named.
func (s *Service) CanEditZone(ctx context.Context, orgID, userID, zoneID int64) (bool, error) {
	access, err := s.Access(ctx, orgID, userID)
	if err != nil {
		return false, err
	}
	if !access.Can(user.PermRecordsWrite) {
		return false, nil
	}
	if access.Role.OutranksOrEquals(user.RoleAdmin) {
		return true, nil
	}

	granted, err := s.store.ZoneGranted(ctx, orgID, userID, zoneID)
	return granted, mapStoreErr(err)
}

// OrgAudit returns an organization's audit trail.
func (s *Service) OrgAudit(ctx context.Context, orgID, actorID int64, limit, offset int) ([]model.AuditEntry, error) {
	if _, err := s.require(ctx, orgID, actorID, user.PermOrgSettings); err != nil {
		return nil, err
	}
	entries, err := s.store.ListOrgAudit(ctx, orgID, limit, offset)
	return entries, mapStoreErr(err)
}

// orgByID reads an organization without a membership check, for the
// paths that have already established access another way.
func (s *Service) orgByID(ctx context.Context, orgID int64) (model.Org, error) {
	org, err := s.store.OrgByID(ctx, orgID)
	return org, mapStoreErr(err)
}

// EventOrgCreatedPersonal names the automatic creation of a personal
// organization, which is distinct from a user starting a shared one.
const EventOrgCreatedPersonal = "org.created_personal"

// audit records an organization event.
//
// A failure to write the trail is deliberately not propagated: the
// action it describes has already succeeded, and unwinding it would be a
// worse outcome than a gap in the log. The gap is instead made visible
// by the details column, which records the marshalling failure.
func (s *Service) audit(ctx context.Context, orgID int64, event, actorType string, actorID, targetID int64, details map[string]any) {
	_ = s.store.RecordOrgAudit(ctx, model.AuditEntry{
		SubjectID: orgID,
		Event:     event,
		ActorType: actorType,
		ActorID:   actorID,
		TargetID:  targetID,
		Details:   encodeDetails(details),
	})
}
