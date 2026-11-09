//go:build darwin

package ui

import (
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// nsGoString must copy: the NSString AppKit hands to a service is
// autoreleased, and its buffer is gone after the callback returns, before the
// delayed main-thread update reads the text.
func TestNSGoStringCopies(t *testing.T) {
	s := nsString("服务测试：abc")
	if s == 0 {
		t.Fatal("nsString returned nil")
	}
	p := objc.Send[*byte](s, selUTF8String)
	got := nsGoString(s)
	if got != "服务测试：abc" {
		t.Fatalf("nsGoString = %q", got)
	}
	if unsafe.StringData(got) == p {
		t.Fatal("nsGoString aliases the NSString buffer instead of copying it")
	}
}
