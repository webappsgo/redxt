package handler

import (
	"net/http"
	"strconv"

	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/user"
)

// maxAuditPage bounds an audit listing. A caller asking for more is
// served this many rows rather than refused, so paging stays simple and
// a single request can never ask the database for the whole trail.
const maxAuditPage = 200

// apiListOrgs lists the organizations the caller belongs to.
func (h *Handler) apiListOrgs(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	orgs, err := h.svc.OrgsForUser(r.Context(), c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, orgViews(orgs))
}

// apiCreateOrg starts a shared organization owned by the caller.
func (h *Handler) apiCreateOrg(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	var body struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Website     string `json:"website"`
		Location    string `json:"location"`
		Invite      string `json:"invite"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Slug = formValue(r, "slug")
		body.Name = formValue(r, "name")
		body.Description = formValue(r, "description")
		body.Website = formValue(r, "website")
		body.Location = formValue(r, "location")
		body.Invite = formValue(r, "invite")
	}

	// ByAdmin is deliberately not read from the body. It marks a
	// Server Admin path, and a Regular User naming it in a request must
	// not be able to take it.
	org, err := h.svc.CreateOrg(r.Context(), c.UserID, service.CreateOrgInput{
		Slug:        body.Slug,
		Name:        body.Name,
		Description: body.Description,
		Website:     body.Website,
		Location:    body.Location,
		InviteCode:  body.Invite,
	})
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newOrgView(org))
}

// apiAcceptInvite joins the caller to an organization from an invite.
func (h *Handler) apiAcceptInvite(w http.ResponseWriter, r *http.Request) {
	c, ok := h.requireSession(w, r)
	if !ok {
		return
	}
	org, err := h.svc.AcceptInvite(r.Context(), c.UserID, r.PathValue("code"))
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newOrgView(org))
}

// apiOrg returns one organization the caller belongs to.
func (h *Handler) apiOrg(w http.ResponseWriter, r *http.Request) {
	_, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	sendOK(w, map[string]any{
		"org":         newOrgView(access.Org),
		"role":        string(access.Role),
		"permissions": access.Role.Permissions(),
	})
}

// apiUpdateOrg writes an organization's profile.
func (h *Handler) apiUpdateOrg(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Website     string `json:"website"`
		Location    string `json:"location"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Name = formValue(r, "name")
		body.Description = formValue(r, "description")
		body.Website = formValue(r, "website")
		body.Location = formValue(r, "location")
	}

	// The slug is not carried in the update. It is the organization's
	// address, and changing it would break every URL and every token
	// scoped through it.
	org, err := h.svc.UpdateOrg(r.Context(), access.Org.ID, c.UserID, service.CreateOrgInput{
		Slug:        access.Org.Slug,
		Name:        body.Name,
		Description: body.Description,
		Website:     body.Website,
		Location:    body.Location,
	})
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newOrgView(org))
}

// apiDeleteOrg removes an organization.
func (h *Handler) apiDeleteOrg(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteOrg(r.Context(), access.Org.ID, c.UserID); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"deleted": true})
}

// apiOrgSettings returns the settings a member may change.
func (h *Handler) apiOrgSettings(w http.ResponseWriter, r *http.Request) {
	_, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	sendOK(w, map[string]any{
		"slug":        access.Org.Slug,
		"name":        access.Org.Name,
		"description": access.Org.Description,
		"website":     access.Org.Website,
		"location":    access.Org.Location,
		"visibility":  access.Org.Visibility,
		"personal":    access.Org.Personal,
		"can_edit":    access.Can(user.PermOrgSettings),
	})
}

// apiUpdateOrgSettings writes an organization's visibility.
func (h *Handler) apiUpdateOrgSettings(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	var body struct {
		Visibility string `json:"visibility"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Visibility = formValue(r, "visibility")
	}

	if err = h.svc.SetOrgVisibility(r.Context(), access.Org.ID, c.UserID, body.Visibility); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"visibility": body.Visibility})
}

// apiTransferOrg hands ownership to another member.
func (h *Handler) apiTransferOrg(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	var body struct {
		OwnerID int64 `json:"owner_id"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.OwnerID, _ = strconv.ParseInt(formValue(r, "owner_id"), 10, 64)
	}

	if err = h.svc.TransferOrg(r.Context(), access.Org.ID, c.UserID, body.OwnerID); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"owner_id": body.OwnerID})
}

// apiMembers lists an organization's members.
func (h *Handler) apiMembers(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	members, err := h.svc.Members(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	out := make([]memberView, 0, len(members))
	for _, m := range members {
		out = append(out, memberView{
			UserID:   m.UserID,
			Username: m.Username,
			Email:    m.Email,
			Role:     m.Role,
			JoinedAt: m.CreatedAt,
		})
	}
	sendOK(w, out)
}

// apiSetMemberRole changes a member's role.
func (h *Handler) apiSetMemberRole(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	target, ok := pathID(r, "member_id")
	if !ok {
		notFound(w)
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Role = formValue(r, "role")
	}

	if err = h.svc.SetMemberRole(r.Context(), access.Org.ID, c.UserID, target, body.Role); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"user_id": target, "role": body.Role})
}

// apiRemoveMember removes a member from an organization.
func (h *Handler) apiRemoveMember(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	target, ok := pathID(r, "member_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RemoveMember(r.Context(), access.Org.ID, c.UserID, target); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"removed": true})
}

// apiListInvites lists an organization's outstanding invitations.
func (h *Handler) apiListInvites(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	invites, err := h.svc.ListInvites(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	out := make([]inviteView, 0, len(invites))
	for _, inv := range invites {
		out = append(out, newInviteView(inv))
	}
	sendOK(w, out)
}

// apiCreateInvite issues an invitation into an organization.
func (h *Handler) apiCreateInvite(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	var body struct {
		Email   string `json:"email"`
		Role    string `json:"role"`
		MaxUses int    `json:"max_uses"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Email = formValue(r, "email")
		body.Role = formValue(r, "role")
		body.MaxUses, _ = strconv.Atoi(formValue(r, "max_uses"))
	}

	result, err := h.svc.InviteMember(r.Context(), access.Org.ID, c.UserID, service.InviteInput{
		Email:   body.Email,
		Role:    body.Role,
		MaxUses: body.MaxUses,
	})
	if err != nil {
		sendErr(w, err)
		return
	}

	// The code appears here once. Only its hash is stored, so a caller
	// that loses it must issue a new invitation.
	sendOK(w, map[string]any{
		"invite": newInviteView(result.Invite),
		"code":   result.Code,
	})
}

// apiRevokeInvite withdraws an outstanding invitation.
func (h *Handler) apiRevokeInvite(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "invite_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RevokeInvite(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"revoked": true})
}

// apiOrgAudit reads an organization's audit trail.
func (h *Handler) apiOrgAudit(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	limit := queryInt(r, "limit", 50)
	if limit <= 0 || limit > maxAuditPage {
		limit = maxAuditPage
	}
	offset := queryInt(r, "offset", 0)
	if offset < 0 {
		offset = 0
	}

	entries, err := h.svc.OrgAudit(r.Context(), access.Org.ID, c.UserID, limit, offset)
	if err != nil {
		sendErr(w, err)
		return
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":         e.ID,
			"event":      e.Event,
			"actor_type": e.ActorType,
			"actor_id":   e.ActorID,
			"target_id":  e.TargetID,
			"details":    e.Details,
			"created_at": e.CreatedAt,
		})
	}
	sendOK(w, out)
}

// queryInt reads a numeric query parameter, falling back to a default
// when it is absent or unparsable.
func queryInt(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

// apiListOrgTokens lists the tokens issued inside an organization.
func (h *Handler) apiListOrgTokens(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	tokens, err := h.svc.ListOrgTokens(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, tokenViews(tokens))
}

// apiRevokeOrgToken revokes a token issued inside an organization.
func (h *Handler) apiRevokeOrgToken(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "token_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RevokeOrgToken(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"revoked": true})
}

// apiGrantZone gives a member access to a single zone.
func (h *Handler) apiGrantZone(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	zoneID, ok := pathID(r, "zone_id")
	if !ok {
		notFound(w)
		return
	}

	var body struct {
		MemberID   int64  `json:"member_id"`
		Permission string `json:"permission"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.MemberID, _ = strconv.ParseInt(formValue(r, "member_id"), 10, 64)
		body.Permission = formValue(r, "permission")
	}

	err = h.svc.GrantZone(r.Context(), access.Org.ID, c.UserID, body.MemberID, zoneID, body.Permission)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{
		"zone_id":    zoneID,
		"member_id":  body.MemberID,
		"permission": body.Permission,
	})
}

// apiRevokeZone withdraws a member's access to a single zone.
func (h *Handler) apiRevokeZone(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	zoneID, ok := pathID(r, "zone_id")
	if !ok {
		notFound(w)
		return
	}
	target, ok := pathID(r, "member_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RevokeZone(r.Context(), access.Org.ID, c.UserID, target, zoneID); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"revoked": true})
}
