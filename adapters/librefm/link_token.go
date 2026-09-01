package librefm

import (
	"errors"
	"time"

	"github.com/navidrome/navidrome/core/auth"
)

const (
	linkTokenScope = "librefm-link"
	linkTokenTTL   = 5 * time.Minute
)

func createLinkToken(userID string) (string, error) {
	claims := map[string]any{
		"uid":   userID,
		"scope": linkTokenScope,
		"exp":   time.Now().Add(linkTokenTTL).UTC().Unix(),
	}
	return auth.EncodeToken(claims)
}

func verifyLinkToken(tokenStr string) (string, error) {
	token, err := auth.DecodeAndVerifyToken(tokenStr)
	if err != nil {
		return "", err
	}
	if exp, ok := token.Expiration(); !ok || exp.IsZero() {
		return "", errors.New("link token missing expiration")
	}
	var scope string
	if err := token.Get("scope", &scope); err != nil || scope != linkTokenScope {
		return "", errors.New("invalid link token scope")
	}
	var uid string
	if err := token.Get("uid", &uid); err != nil || uid == "" {
		return "", errors.New("invalid link token user ID")
	}
	return uid, nil
}
