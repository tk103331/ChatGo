// Package qrscanner opens the platform's native QR scanner. Camera frames
// stay on the phone; only the decoded text is returned to Go.
package qrscanner

import (
	"context"

	"github.com/egoist/mygo"
)

// Scan returns an empty string when the user cancels.
func Scan(ctx context.Context) (string, error) {
	var result struct {
		Text string `json:"text"`
	}
	err := mygo.Native("qrscanner").Call(ctx, "scan", nil, &result)
	return result.Text, err
}
