//go:build darwin

package capture

import (
	"bytes"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// CoreFoundation string encoding for UTF-8.
const cfStringEncodingUTF8 uint32 = 0x08000100

// AXValue types, from AXValue.h.
const (
	axValueCGPointType uint32 = 1
	axValueCGSizeType  uint32 = 2
	axValueCGRectType  uint32 = 3
	axValueCFRangeType uint32 = 4
)

type cgPoint struct{ X, Y float64 }
type cgSize struct{ Width, Height float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}
type cfRange struct {
	Location int64
	Length   int64
}

var (
	nativeOnce sync.Once
	nativeErr  error

	cfStringCreateWithCString         func(alloc uintptr, cStr *byte, encoding uint32) uintptr
	cfStringGetLength                 func(s uintptr) int64
	cfStringGetMaximumSizeForEncoding func(length int64, encoding uint32) int64
	cfStringGetCString                func(s uintptr, buf *byte, size int64, encoding uint32) bool
	cfArrayGetCount                   func(a uintptr) int64
	cfArrayGetValueAtIndex            func(a uintptr, idx int64) uintptr
	cfGetTypeID                       func(cf uintptr) uintptr
	cfURLGetTypeID                    func() uintptr
	cfRelease                         func(cf uintptr)

	axUIElementCreateApplication          func(pid int32) uintptr
	axUIElementCopyAttributeValue         func(el, attr uintptr, out *uintptr) int32
	axUIElementCopyParameterizedAttribute func(el, attr, param uintptr, out *uintptr) int32
	axUIElementSetAttributeValue          func(el, attr, value uintptr) int32
	axUIElementSetMessagingTimeout        func(el uintptr, seconds float32) int32
	axValueGetValue                       func(v uintptr, typ uint32, out unsafe.Pointer) bool
	axValueGetTypeID                      func() uintptr
	axIsProcessTrusted                    func() bool
	axIsProcessTrustedWithOptions         func(opts uintptr) bool
	isSecureEventInputEnabled             func() bool
	cfDictionaryCreate                    func(alloc uintptr, keys, values *uintptr, num int64, keyCB, valueCB uintptr) uintptr
	kCFTypeDictionaryKeyCallBacks         uintptr
	kCFTypeDictionaryValueCallBacks       uintptr
	// kCFBooleanTrue is a CFBooleanRef global, not a function.
	kCFBooleanTrue uintptr

	selAlloc             objc.SEL
	selInit              objc.SEL
	selDrain             objc.SEL
	selSharedWorkspace   objc.SEL
	selFrontmost         objc.SEL
	selProcessIdentifier objc.SEL
	selLocalizedName     objc.SEL
	selBundleIdentifier  objc.SEL
	selUTF8String        objc.SEL
	selAbsoluteString    objc.SEL
	selGeneralPasteboard objc.SEL
	selStringForType     objc.SEL
	selAddGlobalMonitor  objc.SEL
	selRemoveMonitor     objc.SEL
	selStringWithUTF8    objc.SEL
)

// loadNative opens the frameworks and binds the symbols the capture reads
// with. It runs once; a failure leaves every read reporting ErrUnsupported.
func loadNative() error {
	nativeOnce.Do(func() {
		const (
			coreFoundation = "/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation"
			hiServices     = "/System/Library/Frameworks/CoreServices.framework/Frameworks/HIServices.framework/HIServices"
			appServices    = "/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices"
			appKit         = "/System/Library/Frameworks/AppKit.framework/AppKit"
			carbon         = "/System/Library/Frameworks/Carbon.framework/Carbon"
		)
		cf, err := purego.Dlopen(coreFoundation, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			nativeErr = err
			return
		}
		ax, err := purego.Dlopen(hiServices, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			ax, err = purego.Dlopen(appServices, purego.RTLD_GLOBAL|purego.RTLD_NOW)
			if err != nil {
				nativeErr = err
				return
			}
		}
		if _, err := purego.Dlopen(appKit, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
			nativeErr = err
			return
		}

		tryRegister(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		tryRegister(&cfStringGetLength, cf, "CFStringGetLength")
		tryRegister(&cfStringGetMaximumSizeForEncoding, cf, "CFStringGetMaximumSizeForEncoding")
		tryRegister(&cfStringGetCString, cf, "CFStringGetCString")
		tryRegister(&cfArrayGetCount, cf, "CFArrayGetCount")
		tryRegister(&cfArrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
		tryRegister(&cfGetTypeID, cf, "CFGetTypeID")
		tryRegister(&cfURLGetTypeID, cf, "CFURLGetTypeID")
		tryRegister(&cfRelease, cf, "CFRelease")
		tryRegister(&cfDictionaryCreate, cf, "CFDictionaryCreate")

		tryRegister(&axUIElementCreateApplication, ax, "AXUIElementCreateApplication")
		tryRegister(&axUIElementCopyAttributeValue, ax, "AXUIElementCopyAttributeValue")
		tryRegister(&axUIElementCopyParameterizedAttribute, ax, "AXUIElementCopyParameterizedAttributeValue")
		tryRegister(&axUIElementSetAttributeValue, ax, "AXUIElementSetAttributeValue")
		tryRegister(&axUIElementSetMessagingTimeout, ax, "AXUIElementSetMessagingTimeout")
		tryRegister(&axValueGetValue, ax, "AXValueGetValue")
		tryRegister(&axValueGetTypeID, ax, "AXValueGetTypeID")
		tryRegister(&axIsProcessTrusted, ax, "AXIsProcessTrusted")
		tryRegister(&axIsProcessTrustedWithOptions, ax, "AXIsProcessTrustedWithOptions")

		// A CFBooleanRef global: Dlsym returns the address of the variable,
		// which holds the object pointer.
		if p, err := purego.Dlsym(cf, "kCFBooleanTrue"); err == nil {
			kCFBooleanTrue = **(**uintptr)(unsafe.Pointer(&p))
		}
		// Callback structs are themselves the symbol, so their address is
		// what CFDictionaryCreate takes.
		if p, err := purego.Dlsym(cf, "kCFTypeDictionaryKeyCallBacks"); err == nil {
			kCFTypeDictionaryKeyCallBacks = p
		}
		if p, err := purego.Dlsym(cf, "kCFTypeDictionaryValueCallBacks"); err == nil {
			kCFTypeDictionaryValueCallBacks = p
		}

		// Secure input is optional: without it the panel may read a password
		// field that hides its subrole. Never fail the whole feature for it.
		if lib, err := purego.Dlopen(carbon, purego.RTLD_GLOBAL|purego.RTLD_NOW); err == nil {
			if p, err := purego.Dlsym(lib, "IsSecureEventInputEnabled"); err == nil {
				purego.RegisterFunc(&isSecureEventInputEnabled, p)
			}
		}
		if isSecureEventInputEnabled == nil {
			isSecureEventInputEnabled = func() bool { return false }
		}

		selAlloc = objc.RegisterName("alloc")
		selInit = objc.RegisterName("init")
		selDrain = objc.RegisterName("drain")
		selSharedWorkspace = objc.RegisterName("sharedWorkspace")
		selFrontmost = objc.RegisterName("frontmostApplication")
		selProcessIdentifier = objc.RegisterName("processIdentifier")
		selLocalizedName = objc.RegisterName("localizedName")
		selBundleIdentifier = objc.RegisterName("bundleIdentifier")
		selUTF8String = objc.RegisterName("UTF8String")
		selAbsoluteString = objc.RegisterName("absoluteString")
		selGeneralPasteboard = objc.RegisterName("generalPasteboard")
		selStringForType = objc.RegisterName("stringForType:")
		selAddGlobalMonitor = objc.RegisterName("addGlobalMonitorForEventsMatchingMask:handler:")
		selRemoveMonitor = objc.RegisterName("removeMonitor:")
		selStringWithUTF8 = objc.RegisterName("stringWithUTF8String:")
	})
	return nativeErr
}

// tryRegister binds one C symbol, remembering the first that is missing
// instead of panicking: a partial bridge then reports ErrUnsupported.
func tryRegister(fptr any, lib uintptr, name string) {
	if nativeErr != nil {
		return
	}
	p, err := purego.Dlsym(lib, name)
	if err != nil {
		nativeErr = fmt.Errorf("capture: %s: %w", name, err)
		return
	}
	purego.RegisterFunc(fptr, p)
}

// cfString makes a CFString for an AX attribute name. The caller releases it.
func cfString(s string) uintptr {
	if s == "" {
		return 0
	}
	buf := make([]byte, len(s)+1)
	copy(buf, s)
	return cfStringCreateWithCString(0, &buf[0], cfStringEncodingUTF8)
}

// cfStringToGo copies a CFString into a Go string.
func cfStringToGo(s uintptr) string {
	if s == 0 {
		return ""
	}
	n := cfStringGetLength(s)
	if n <= 0 {
		return ""
	}
	size := cfStringGetMaximumSizeForEncoding(n, cfStringEncodingUTF8) + 1
	if size <= 1 {
		return ""
	}
	buf := make([]byte, size)
	if !cfStringGetCString(s, &buf[0], size, cfStringEncodingUTF8) {
		return ""
	}
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		return string(buf[:i])
	}
	return string(buf)
}

// nsString makes an autoreleased NSString for selectors that take one.
func nsString(s string) objc.ID {
	if s == "" {
		return 0
	}
	return objc.Send[objc.ID](objc.ID(objc.GetClass("NSString")), selStringWithUTF8, s)
}

// goString copies an NSString into a Go string.
func goString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	p := objc.Send[*byte](id, selUTF8String)
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	// Copy: p points into an autoreleased NSString, whose buffer is freed
	// when the pool drains, while the Selection outlives it.
	return string(unsafe.Slice(p, n))
}

// withPool runs fn inside an autorelease pool, which matters on the
// goroutines that read a selection: they run outside the app's own loop.
func withPool(fn func()) {
	if loadNative() != nil {
		fn()
		return
	}
	cls := objc.GetClass("NSAutoreleasePool")
	if cls == 0 {
		fn()
		return
	}
	pool := objc.Send[objc.ID](objc.Send[objc.ID](objc.ID(cls), selAlloc), selInit)
	if pool == 0 {
		fn()
		return
	}
	defer objc.Send[objc.ID](pool, selDrain)
	fn()
}

// copyAttr returns a +1 reference to an AX attribute value, or 0.
func copyAttr(el uintptr, name string) uintptr {
	key := cfString(name)
	if key == 0 {
		return 0
	}
	defer cfRelease(key)
	var out uintptr
	if code := axUIElementCopyAttributeValue(el, key, &out); code != 0 {
		debugf("AX %s failed: %d", name, code)
		return 0
	}
	return out
}

// attrString reads a string attribute.
func attrString(el uintptr, name string) string {
	v := copyAttr(el, name)
	if v == 0 {
		return ""
	}
	defer cfRelease(v)
	return cfStringToGo(v)
}

// attrURL reads a CFURL attribute, such as Finder's AXURL.
func attrURL(el uintptr) string {
	v := copyAttr(el, "AXURL")
	if v == 0 {
		return ""
	}
	defer cfRelease(v)
	if cfGetTypeID(v) != cfURLGetTypeID() {
		return ""
	}
	return goString(objc.Send[objc.ID](objc.ID(v), selAbsoluteString))
}

// attrPoint and attrSize read AX geometry values.
func attrPoint(el uintptr, name string) (cgPoint, bool) {
	v := copyAttr(el, name)
	if v == 0 {
		return cgPoint{}, false
	}
	defer cfRelease(v)
	if cfGetTypeID(v) != axValueGetTypeID() {
		return cgPoint{}, false
	}
	var p cgPoint
	if !axValueGetValue(v, axValueCGPointType, unsafe.Pointer(&p)) {
		return cgPoint{}, false
	}
	return p, true
}

func attrSize(el uintptr, name string) (cgSize, bool) {
	v := copyAttr(el, name)
	if v == 0 {
		return cgSize{}, false
	}
	defer cfRelease(v)
	if cfGetTypeID(v) != axValueGetTypeID() {
		return cgSize{}, false
	}
	var s cgSize
	if !axValueGetValue(v, axValueCGSizeType, unsafe.Pointer(&s)) {
		return cgSize{}, false
	}
	return s, true
}

// frontmostApp describes the application the user is in.
func frontmostApp() (pid int32, name, bundle string) {
	ws := objc.Send[objc.ID](objc.ID(objc.GetClass("NSWorkspace")), selSharedWorkspace)
	if ws == 0 {
		return 0, "", ""
	}
	app := objc.Send[objc.ID](ws, selFrontmost)
	if app == 0 {
		return 0, "", ""
	}
	pid = objc.Send[int32](app, selProcessIdentifier)
	name = goString(objc.Send[objc.ID](app, selLocalizedName))
	bundle = goString(objc.Send[objc.ID](app, selBundleIdentifier))
	return pid, name, bundle
}

// pasteboardString reads a string type from the general pasteboard.
func pasteboardString(pasteboardType string) string {
	pb := objc.Send[objc.ID](objc.ID(objc.GetClass("NSPasteboard")), selGeneralPasteboard)
	if pb == 0 {
		return ""
	}
	value := objc.Send[objc.ID](pb, selStringForType, nsString(pasteboardType))
	return goString(value)
}
