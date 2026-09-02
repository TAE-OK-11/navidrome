//go:build windows || plan9

package cmd

import (
	"context"

	"github.com/navidrome/navidrome/model"
)

// Windows and Plan9 don't support SIGUSR1, so we don't need to start a signaler
func startSignaller(_ context.Context, _ model.Scanner) func() error {
	return func() error {
		return nil
	}
}
