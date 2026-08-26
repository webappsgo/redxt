package handler

import (
	"net/http"

	"github.com/webappsgo/redxt/src/server/service"
)

// apiListDomains lists an organization's custom domains.
func (h *Handler) apiListDomains(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	domains, err := h.svc.ListDomains(r.Context(), access.Org.ID, c.UserID)
	if err != nil {
		sendErr(w, err)
		return
	}

	out := make([]domainView, 0, len(domains))
	for _, d := range domains {
		out = append(out, newDomainView(d))
	}
	sendOK(w, out)
}

// apiAddDomain claims a custom domain for an organization.
//
// The domain is recorded unverified and serves nothing until ownership
// is proved, so claiming a name another operator controls gains the
// caller no reach over it.
func (h *Handler) apiAddDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}

	var body struct {
		Domain  string `json:"domain"`
		Purpose string `json:"purpose"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Domain = formValue(r, "domain")
		body.Purpose = formValue(r, "purpose")
	}

	domain, err := h.svc.AddDomain(r.Context(), access.Org.ID, c.UserID, service.AddDomainInput{
		Domain:  body.Domain,
		Purpose: body.Purpose,
	})
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newDomainView(domain))
}

// apiUpdateDomain changes what a verified domain is used for.
func (h *Handler) apiUpdateDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		notFound(w)
		return
	}

	var body struct {
		Purpose string `json:"purpose"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Purpose = formValue(r, "purpose")
	}

	err = h.svc.SetDomainPurpose(r.Context(), access.Org.ID, c.UserID, id, body.Purpose)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"id": id, "purpose": body.Purpose})
}

// apiRemoveDomain releases a custom domain.
func (h *Handler) apiRemoveDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		notFound(w)
		return
	}
	if err := h.svc.RemoveDomain(r.Context(), access.Org.ID, c.UserID, id); err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"removed": true})
}

// apiDomainVerification returns the DNS record proving ownership.
func (h *Handler) apiDomainVerification(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		notFound(w)
		return
	}

	record, err := h.svc.VerificationInstructions(r.Context(), access.Org.ID, c.UserID, id)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{
		"name":  record.Name,
		"type":  record.Type,
		"value": record.Value,
	})
}

// apiVerifyDomain checks the published record and activates the domain.
//
// A certificate is requested from the server's existing ACME manager as
// part of that activation, so a custom domain is served over the same
// TLS path as every other name this server answers for.
func (h *Handler) apiVerifyDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		notFound(w)
		return
	}

	domain, err := h.svc.VerifyDomain(r.Context(), access.Org.ID, c.UserID, id)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, newDomainView(domain))
}

// apiSuspendDomain stops serving a domain without releasing it.
func (h *Handler) apiSuspendDomain(w http.ResponseWriter, r *http.Request) {
	c, access, ok := h.orgScope(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "domain_id")
	if !ok {
		notFound(w)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	form, err := bind(r, &body)
	if err != nil {
		badRequest(w)
		return
	}
	if form {
		body.Reason = formValue(r, "reason")
	}

	err = h.svc.SuspendDomain(r.Context(), access.Org.ID, c.UserID, id, body.Reason)
	if err != nil {
		sendErr(w, err)
		return
	}
	sendOK(w, map[string]any{"suspended": true})
}
