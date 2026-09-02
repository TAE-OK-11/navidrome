package subsonic

import (
	"net/http"

	"github.com/navidrome/navidrome/model/request"
	"github.com/navidrome/navidrome/server/subsonic/responses"
)

func (api *Router) GetTokenInfo(r *http.Request) (*responses.Subsonic, error) {
	user, ok := request.UserFrom(r.Context())
	if !ok {
		return nil, newErrorWithHelp(responses.ErrorInvalidAPIKey, apiKeyHelpURL)
	}
	response := newResponse()
	response.TokenInfo = &responses.TokenInfo{Username: user.UserName}
	return response, nil
}
