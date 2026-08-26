package notify

import "time"

// GlobalVarInput carries the values Vars needs to build the "Global
// Variables (Available in All Templates)" map from AI.md PART 18. It
// is a plain struct, not an import of the config package, so notify
// stays independently testable and free of any import-cycle risk.
type GlobalVarInput struct {
	AppName           string
	AppURL            string
	FQDN              string
	AdminEmail        string
	RecipientEmail    string
	RecipientUsername string
	// OnionAddress is Tor.OnionAddress from config; empty when the Tor
	// hidden service (PART 32.1) is not enabled or not yet assigned.
	OnionAddress string
	// Now is injectable so tests never depend on the wall clock.
	Now time.Time
}

// Vars builds the {variable} substitution map every template may use,
// per AI.md PART 18's Global Variables table. i2p_url and i2p_address
// are always empty: I2P eepsite support (PART 32.2) is optional and
// not implemented in this project, so there is no address to expose.
func Vars(in GlobalVarInput) map[string]string {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	onionURL := ""
	if in.OnionAddress != "" {
		onionURL = "http://" + in.OnionAddress
	}

	return map[string]string{
		"app_name":           in.AppName,
		"app_url":            in.AppURL,
		"fqdn":               in.FQDN,
		"onion_url":          onionURL,
		"onion_address":      in.OnionAddress,
		"i2p_url":            "",
		"i2p_address":        "",
		"admin_email":        in.AdminEmail,
		"recipient_email":    in.RecipientEmail,
		"recipient_username": in.RecipientUsername,
		"timestamp":          now.Format(time.RFC1123),
		"year":               now.Format("2006"),
	}
}

// Merge layers extra on top of base, returning a new map. Callers use
// it to combine Vars' global map with a template's own
// TemplateVariables values (e.g. {reset_link}, {ip}) before calling
// Render.
func Merge(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
