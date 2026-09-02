package subsonic

import (
	"github.com/navidrome/navidrome/server/subsonic/responses"
	"github.com/navidrome/navidrome/utils/req"
)

const apiKeyHelpURL = "https://www.navidrome.org/docs/getting-started/login/#api-access"

func hasAPIKeyAuth(p *req.Values) bool {
	return p.StringOr("apiKey", "") != ""
}

func validateAPIKeyAuthParams(p *req.Values) error {
	if p.StringOr("u", "") != "" {
		return newErrorWithHelp(responses.ErrorConflictingAuth, apiKeyHelpURL)
	}
	if p.StringOr("p", "") != "" || p.StringOr("t", "") != "" || p.StringOr("s", "") != "" {
		return newErrorWithHelp(responses.ErrorConflictingAuth, apiKeyHelpURL)
	}
	if p.StringOr("jwt", "") != "" {
		return newErrorWithHelp(responses.ErrorConflictingAuth, apiKeyHelpURL)
	}
	return nil
}

func validatePasswordAuthParams(p *req.Values) error {
	if hasAPIKeyAuth(p) {
		return newErrorWithHelp(responses.ErrorConflictingAuth, apiKeyHelpURL)
	}
	return nil
}
