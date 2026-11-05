// Package config holds deployment facts that are stable for the lifetime of
// the process and do not belong in the database. Legal identity is env-only so
// a public privacy policy cannot be changed by an application admin.
package config

import (
	"os"
	"strings"
)

// Legal is the identity of the instance operator used by the public legal
// pages. Empty values are allowed; the handler falls back to neutral text
// rather than inventing a contact address.
type Legal struct {
	ControllerName       string
	ContactEmail         string
	ContactAddress       string
	HostingRegion        string
	DPOEmail             string
	EURepresentative     string
	SupervisoryAuthority string
}

// LoadLegal reads the LEGAL_* environment variables. The values are read once
// at startup; changing them requires a restart.
func LoadLegal() Legal {
	return Legal{
		ControllerName:       strings.TrimSpace(os.Getenv("LEGAL_CONTROLLER_NAME")),
		ContactEmail:         strings.TrimSpace(os.Getenv("LEGAL_CONTACT_EMAIL")),
		ContactAddress:       strings.TrimSpace(os.Getenv("LEGAL_CONTACT_ADDRESS")),
		HostingRegion:        strings.TrimSpace(os.Getenv("LEGAL_HOSTING_REGION")),
		DPOEmail:             strings.TrimSpace(os.Getenv("LEGAL_DPO_EMAIL")),
		EURepresentative:     strings.TrimSpace(os.Getenv("LEGAL_EU_REPRESENTATIVE")),
		SupervisoryAuthority: strings.TrimSpace(os.Getenv("LEGAL_SUPERVISORY_AUTHORITY")),
	}
}
