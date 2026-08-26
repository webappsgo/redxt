package user

import "errors"

// Role is a member's role inside an organization. AI.md PART 35 defines
// a generic Owner/Admin/Member set; IDEA.md refines the third tier into
// Editor and Viewer for redxt's DNS permission table, which is the
// authoritative set for this project.
type Role string

const (
	// RoleOwner may do everything, including transferring and deleting
	// the organization.
	RoleOwner Role = "owner"
	// RoleAdmin manages zones, records, keys, tokens, DDNS hosts, and org
	// policies, and manages members below admin. It cannot delete or
	// transfer the organization.
	RoleAdmin Role = "admin"
	// RoleEditor creates and edits records, DDNS hosts, and scheduled
	// changes in assigned zones. It cannot export keys and cannot manage
	// members or the zone lifecycle.
	RoleEditor Role = "editor"
	// RoleViewer reads zones, records, and org-scoped analytics.
	RoleViewer Role = "viewer"
)

// ErrUnknownRole reports a role outside the four IDEA.md values.
var ErrUnknownRole = errors.New("user: unknown organization role")

// Permission names one action a role may or may not perform. The set is
// the IDEA.md "Roles & permissions" table expressed as capabilities, so
// a handler asks about the action rather than about the role.
type Permission string

const (
	// PermRead covers reading zones, records, and org-scoped analytics.
	PermRead Permission = "org:read"
	// PermRecordsWrite covers creating and editing records, DDNS hosts,
	// and scheduled changes. For an editor it applies only inside a
	// granted zone; the grant check is separate and additional.
	PermRecordsWrite Permission = "records:write"
	// PermZonesManage covers the zone lifecycle: create, delete, transfer.
	PermZonesManage Permission = "zones:manage"
	// PermKeysManage covers creating and rotating TSIG and DNSSEC keys.
	PermKeysManage Permission = "keys:manage"
	// PermKeysExport covers reading key material out of the server.
	PermKeysExport Permission = "keys:export"
	// PermTokensManage covers issuing and revoking org API tokens.
	PermTokensManage Permission = "tokens:manage"
	// PermMembersManage covers inviting, removing, and re-roling members.
	PermMembersManage Permission = "members:manage"
	// PermOrgSettings covers editing the organization profile and policy.
	PermOrgSettings Permission = "org:settings"
	// PermOrgTransfer covers handing ownership to another member.
	PermOrgTransfer Permission = "org:transfer"
	// PermOrgDelete covers deleting the organization.
	PermOrgDelete Permission = "org:delete"
)

// rolePermissions is the IDEA.md table. A role holds exactly the
// permissions listed here and nothing is inherited implicitly, so a new
// permission is denied everywhere until it is deliberately granted.
var rolePermissions = map[Role]map[Permission]bool{
	RoleOwner: {
		PermRead: true, PermRecordsWrite: true, PermZonesManage: true,
		PermKeysManage: true, PermKeysExport: true, PermTokensManage: true,
		PermMembersManage: true, PermOrgSettings: true,
		PermOrgTransfer: true, PermOrgDelete: true,
	},
	RoleAdmin: {
		PermRead: true, PermRecordsWrite: true, PermZonesManage: true,
		PermKeysManage: true, PermKeysExport: true, PermTokensManage: true,
		PermMembersManage: true, PermOrgSettings: true,
	},
	RoleEditor: {
		PermRead: true, PermRecordsWrite: true,
	},
	RoleViewer: {
		PermRead: true,
	},
}

// roleRank orders the roles so a member can be compared against another
// without hard-coding pairs. A higher rank is more privileged.
var roleRank = map[Role]int{
	RoleViewer: 1,
	RoleEditor: 2,
	RoleAdmin:  3,
	RoleOwner:  4,
}

// ParseRole resolves a stored or submitted role string.
func ParseRole(raw string) (Role, error) {
	role := Role(NormalizeName(raw))
	if _, known := rolePermissions[role]; !known {
		return "", ErrUnknownRole
	}
	return role, nil
}

// Can reports whether the role holds the permission. An unknown role
// holds nothing, so a corrupted row fails closed.
func (r Role) Can(p Permission) bool {
	return rolePermissions[r][p]
}

// Rank returns the privilege ordering of the role, or zero when the role
// is unknown.
func (r Role) Rank() int {
	return roleRank[r]
}

// OutranksOrEquals reports whether r is at least as privileged as other.
func (r Role) OutranksOrEquals(other Role) bool {
	return r.Rank() >= other.Rank()
}

// CanManageMember reports whether r may create, remove, or re-role a
// member currently holding target. PART 35 lets an admin manage members
// below admin only, which falls out of a strict rank comparison; an
// owner may manage anyone, including another admin.
func (r Role) CanManageMember(target Role) bool {
	if !r.Can(PermMembersManage) {
		return false
	}
	if r == RoleOwner {
		return true
	}
	return r.Rank() > target.Rank()
}

// CanGrantRole reports whether r may hand out the role target. A member
// can never grant a role above their own, which is the rule that stops
// an admin from promoting themselves through a second account.
func (r Role) CanGrantRole(target Role) bool {
	if !r.Can(PermMembersManage) {
		return false
	}
	if target == RoleOwner {
		return r == RoleOwner
	}
	return r.Rank() > target.Rank() || r == RoleOwner
}

// Permissions returns the role's permissions in a stable order suitable
// for an API response.
func (r Role) Permissions() []Permission {
	ordered := []Permission{
		PermRead, PermRecordsWrite, PermZonesManage, PermKeysManage,
		PermKeysExport, PermTokensManage, PermMembersManage,
		PermOrgSettings, PermOrgTransfer, PermOrgDelete,
	}
	held := make([]Permission, 0, len(ordered))
	for _, p := range ordered {
		if r.Can(p) {
			held = append(held, p)
		}
	}
	return held
}

// CapRole clamps requested to the issuer's own role, implementing the
// IDEA.md rule that a credential can never exceed the authority of the
// member who issued it.
func CapRole(issuer, requested Role) Role {
	if requested.Rank() == 0 || requested.Rank() > issuer.Rank() {
		return issuer
	}
	return requested
}
