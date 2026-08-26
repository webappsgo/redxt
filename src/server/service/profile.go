package service

import (
	"context"

	"github.com/webappsgo/redxt/src/server/model"
)

// PublicProfile is what a vanity URL shows about a user.
//
// It is assembled rather than returned as the stored row, so a field the
// owner has chosen not to publish is absent from the value entirely and
// cannot leak through a later change to how profiles are rendered.
type PublicProfile struct {
	Username    string
	DisplayName string
	Bio         string
	Location    string
	Website     string
	AvatarURL   string
	// Email is set only when the owner asked for it to be shown.
	Email string
	// Orgs lists the organizations the owner asked to show.
	Orgs []model.Org
}

// PublicOrgProfile is what an organization's vanity URL shows.
type PublicOrgProfile struct {
	Slug        string
	Name        string
	Description string
	Website     string
	Location    string
	AvatarURL   string
	MemberCount int
}

// UserProfile resolves a vanity URL for a viewer.
//
// A profile the viewer may not see is reported as not found rather than
// forbidden. Answering "forbidden" would confirm the account exists,
// which is exactly what a private profile is meant to withhold.
func (s *Service) UserProfile(ctx context.Context, username string, viewerID int64) (PublicProfile, error) {
	account, err := s.store.UserByUsername(ctx, username)
	if err != nil {
		return PublicProfile{}, ErrNotFound
	}
	if !account.Active() {
		return PublicProfile{}, ErrNotFound
	}

	visible := account.Public() || account.ID == viewerID
	// A private profile still shows to people the owner shares an
	// organization with, when they left org visibility on. That is a
	// deliberate narrowing of "private", not an exception to it: the
	// viewer already knows the account exists.
	if !visible && account.OrgVisibility && viewerID != 0 {
		shared, sharedErr := s.sharesOrg(ctx, account.ID, viewerID)
		if sharedErr != nil {
			return PublicProfile{}, sharedErr
		}
		visible = shared
	}
	if !visible {
		return PublicProfile{}, ErrNotFound
	}

	prefs, err := s.store.Preferences(ctx, account.ID)
	if err != nil {
		return PublicProfile{}, mapStoreErr(err)
	}

	profile := PublicProfile{
		Username:    account.Username,
		DisplayName: account.DisplayName,
		Bio:         account.Bio,
		Location:    account.Location,
		Website:     account.Website,
		AvatarURL:   account.AvatarURL,
	}
	if prefs.ShowEmail {
		profile.Email = account.ContactEmail()
	}
	if prefs.ShowOrgs {
		orgs, orgErr := s.store.OrgsForUser(ctx, account.ID)
		if orgErr != nil {
			return PublicProfile{}, mapStoreErr(orgErr)
		}
		for _, org := range orgs {
			// A personal organization is an implementation detail of how
			// resources are owned, not something the user chose to join,
			// so it is never listed on a profile.
			if org.Personal || !org.Public() {
				continue
			}
			profile.Orgs = append(profile.Orgs, org)
		}
	}

	return profile, nil
}

// OrgProfile resolves an organization's vanity URL for a viewer.
func (s *Service) OrgProfile(ctx context.Context, slug string, viewerID int64) (PublicOrgProfile, error) {
	org, err := s.store.OrgBySlug(ctx, slug)
	if err != nil {
		return PublicOrgProfile{}, ErrNotFound
	}
	if !org.Active() || org.Personal {
		return PublicOrgProfile{}, ErrNotFound
	}

	if !org.Public() {
		if viewerID == 0 {
			return PublicOrgProfile{}, ErrNotFound
		}
		if _, memberErr := s.store.Membership(ctx, org.ID, viewerID); memberErr != nil {
			return PublicOrgProfile{}, ErrNotFound
		}
	}

	count, err := s.store.CountMembers(ctx, org.ID)
	if err != nil {
		return PublicOrgProfile{}, mapStoreErr(err)
	}

	return PublicOrgProfile{
		Slug:        org.Slug,
		Name:        org.Name,
		Description: org.Description,
		Website:     org.Website,
		Location:    org.Location,
		AvatarURL:   org.AvatarURL,
		MemberCount: count,
	}, nil
}

// sharesOrg reports whether two accounts belong to a common
// organization.
func (s *Service) sharesOrg(ctx context.Context, a, b int64) (bool, error) {
	mine, err := s.store.OrgsForUser(ctx, a)
	if err != nil {
		return false, mapStoreErr(err)
	}
	for _, org := range mine {
		if org.Personal {
			continue
		}
		if _, memberErr := s.store.Membership(ctx, org.ID, b); memberErr == nil {
			return true, nil
		}
	}
	return false, nil
}
