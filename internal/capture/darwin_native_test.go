//go:build darwin

package capture

import (
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// goString must copy: app names and URLs read from AX come from autoreleased
// objects, while the Selection outlives the pool they belong to.
func TestGoStringCopies(t *testing.T) {
	if err := loadNative(); err != nil {
		t.Skipf("native bridge unavailable: %v", err)
	}
	s := nsString("服务测试：abc")
	if s == 0 {
		t.Fatal("nsString returned nil")
	}
	p := objc.Send[*byte](s, selUTF8String)
	got := goString(s)
	if got != "服务测试：abc" {
		t.Fatalf("goString = %q", got)
	}
	if unsafe.StringData(got) == p {
		t.Fatal("goString aliases the NSString buffer instead of copying it")
	}
}
