//go:build !darwin && !linux && !windows

package capture

func platformCapture() Capture { return clipboardCapture{} }
