package user

import (
	"errors"
	"testing"
)

func TestParseRole(t *testing.T) {
	tests := []struct {
		input string
		want  Role
		err   error
	}{
		{input: "owner", want: RoleOwner},
		{input: "Admin", want: RoleAdmin},
		{input: " editor ", want: RoleEditor},
		{input: "viewer", want: RoleViewer},
		{input: "member", err: ErrUnknownRole},
		{input: "", err: ErrUnknownRole},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseRole(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ParseRole(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if got != tc.want {
				t.Fatalf("ParseRole(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRoleCan(t *testing.T) {
	tests := []struct {
		name string
		role Role
		perm Permission
		want bool
	}{
		{name: "owner deletes org", role: RoleOwner, perm: PermOrgDelete, want: true},
		{name: "admin cannot delete org", role: RoleAdmin, perm: PermOrgDelete},
		{name: "admin cannot transfer org", role: RoleAdmin, perm: PermOrgTransfer},
		{name: "admin manages tokens", role: RoleAdmin, perm: PermTokensManage, want: true},
		{name: "editor writes records", role: RoleEditor, perm: PermRecordsWrite, want: true},
		{name: "editor cannot export keys", role: RoleEditor, perm: PermKeysExport},
		{name: "editor cannot manage members", role: RoleEditor, perm: PermMembersManage},
		{name: "editor cannot manage zones", role: RoleEditor, perm: PermZonesManage},
		{name: "viewer reads", role: RoleViewer, perm: PermRead, want: true},
		{name: "viewer cannot write", role: RoleViewer, perm: PermRecordsWrite},
		{name: "unknown role holds nothing", role: Role("bogus"), perm: PermRead},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.role.Can(tc.perm); got != tc.want {
				t.Fatalf("%q.Can(%q) = %v, want %v", tc.role, tc.perm, got, tc.want)
			}
		})
	}
}

func TestRoleCanManageMember(t *testing.T) {
	tests := []struct {
		name   string
		actor  Role
		target Role
		want   bool
	}{
		{name: "owner manages admin", actor: RoleOwner, target: RoleAdmin, want: true},
		{name: "owner manages owner", actor: RoleOwner, target: RoleOwner, want: true},
		{name: "admin manages editor", actor: RoleAdmin, target: RoleEditor, want: true},
		{name: "admin cannot manage admin", actor: RoleAdmin, target: RoleAdmin},
		{name: "admin cannot manage owner", actor: RoleAdmin, target: RoleOwner},
		{name: "editor manages nobody", actor: RoleEditor, target: RoleViewer},
		{name: "viewer manages nobody", actor: RoleViewer, target: RoleViewer},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.actor.CanManageMember(tc.target); got != tc.want {
				t.Fatalf("%q.CanManageMember(%q) = %v, want %v", tc.actor, tc.target, got, tc.want)
			}
		})
	}
}

func TestRoleCanGrantRole(t *testing.T) {
	tests := []struct {
		name   string
		actor  Role
		target Role
		want   bool
	}{
		{name: "owner grants owner", actor: RoleOwner, target: RoleOwner, want: true},
		{name: "owner grants admin", actor: RoleOwner, target: RoleAdmin, want: true},
		{name: "admin cannot grant owner", actor: RoleAdmin, target: RoleOwner},
		{name: "admin cannot grant admin", actor: RoleAdmin, target: RoleAdmin},
		{name: "admin grants editor", actor: RoleAdmin, target: RoleEditor, want: true},
		{name: "editor grants nothing", actor: RoleEditor, target: RoleViewer},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.actor.CanGrantRole(tc.target); got != tc.want {
				t.Fatalf("%q.CanGrantRole(%q) = %v, want %v", tc.actor, tc.target, got, tc.want)
			}
		})
	}
}

func TestCapRole(t *testing.T) {
	tests := []struct {
		name      string
		issuer    Role
		requested Role
		want      Role
	}{
		{name: "narrower survives", issuer: RoleAdmin, requested: RoleViewer, want: RoleViewer},
		{name: "equal survives", issuer: RoleEditor, requested: RoleEditor, want: RoleEditor},
		{name: "wider is clamped", issuer: RoleEditor, requested: RoleOwner, want: RoleEditor},
		{name: "unknown is clamped", issuer: RoleViewer, requested: Role("root"), want: RoleViewer},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CapRole(tc.issuer, tc.requested); got != tc.want {
				t.Fatalf("CapRole(%q, %q) = %q, want %q", tc.issuer, tc.requested, got, tc.want)
			}
		})
	}
}

func TestRolePermissions(t *testing.T) {
	if got := len(RoleViewer.Permissions()); got != 1 {
		t.Fatalf("viewer permission count = %d, want 1", got)
	}
	if got := len(RoleOwner.Permissions()); got != 10 {
		t.Fatalf("owner permission count = %d, want 10", got)
	}
}

func TestModes(t *testing.T) {
	registration := []struct {
		name        string
		input       string
		mode        RegistrationMode
		selfService bool
		invite      bool
		adminCreate bool
		err         error
	}{
		{name: "open", input: "open", mode: RegistrationOpen, selfService: true, invite: true, adminCreate: true},
		{name: "invite", input: "invite", mode: RegistrationInvite, invite: true, adminCreate: true},
		{name: "admin only", input: "admin_only", mode: RegistrationAdminOnly, adminCreate: true},
		{name: "disabled", input: "disabled", mode: RegistrationDisabled},
		{name: "unknown", input: "sometimes", err: ErrUnknownMode},
	}

	for _, tc := range registration {
		t.Run("registration/"+tc.name, func(t *testing.T) {
			mode, err := ParseRegistrationMode(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ParseRegistrationMode(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if mode != tc.mode {
				t.Fatalf("ParseRegistrationMode(%q) = %q, want %q", tc.input, mode, tc.mode)
			}
			if got := mode.SelfServiceAllowed(); got != tc.selfService {
				t.Fatalf("%q.SelfServiceAllowed() = %v, want %v", mode, got, tc.selfService)
			}
			if got := mode.InviteAllowed(); got != tc.invite {
				t.Fatalf("%q.InviteAllowed() = %v, want %v", mode, got, tc.invite)
			}
			if got := mode.AdminCreateAllowed(); got != tc.adminCreate {
				t.Fatalf("%q.AdminCreateAllowed() = %v, want %v", mode, got, tc.adminCreate)
			}
		})
	}

	creation := []struct {
		name        string
		input       string
		mode        CreationMode
		selfService bool
		invite      bool
		adminCreate bool
		err         error
	}{
		{name: "open", input: "open", mode: CreationOpen, selfService: true, invite: true, adminCreate: true},
		{name: "invite", input: "invite", mode: CreationInvite, invite: true, adminCreate: true},
		{name: "admin only", input: "admin_only", mode: CreationAdminOnly, adminCreate: true},
		{name: "disabled", input: "disabled", mode: CreationDisabled},
		{name: "unknown", input: "maybe", err: ErrUnknownMode},
	}

	for _, tc := range creation {
		t.Run("creation/"+tc.name, func(t *testing.T) {
			mode, err := ParseCreationMode(tc.input)
			if !errors.Is(err, tc.err) {
				t.Fatalf("ParseCreationMode(%q) error = %v, want %v", tc.input, err, tc.err)
			}
			if mode != tc.mode {
				t.Fatalf("ParseCreationMode(%q) = %q, want %q", tc.input, mode, tc.mode)
			}
			if got := mode.SelfServiceAllowed(); got != tc.selfService {
				t.Fatalf("%q.SelfServiceAllowed() = %v, want %v", mode, got, tc.selfService)
			}
			if got := mode.InviteAllowed(); got != tc.invite {
				t.Fatalf("%q.InviteAllowed() = %v, want %v", mode, got, tc.invite)
			}
			if got := mode.AdminCreateAllowed(); got != tc.adminCreate {
				t.Fatalf("%q.AdminCreateAllowed() = %v, want %v", mode, got, tc.adminCreate)
			}
		})
	}
}
