//go:build darwin

package ui

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The app's Services menu entry ("向 ChatGo 提问") is declared by NSServices
// in mygo.json, which `mygo build` writes into Info.plist. AppKit sends the
// selection to the object registered here with -[NSApplication
// setServicesProvider:], calling askChatGo:userData:error:.
var (
	serviceOnce     sync.Once
	serviceTarget   *ChatWindow
	serviceProvider objc.ID
)

var (
	selSharedApplication   = objc.RegisterName("sharedApplication")
	selSetServicesProvider = objc.RegisterName("setServicesProvider:")
	selNew                 = objc.RegisterName("new")
	selStringForType       = objc.RegisterName("stringForType:")
	selUTF8String          = objc.RegisterName("UTF8String")
	selStringWithUTF8      = objc.RegisterName("stringWithUTF8String:")
)

// installServiceProvider registers the Services provider. It must run on the
// main thread, before the system delivers a service request, and does nothing
// when called again.
func installServiceProvider(cw *ChatWindow) {
	serviceOnce.Do(func() {
		serviceTarget = cw
		className := "ChatGoServiceProvider"
		cls := objc.GetClass(className)
		if cls == 0 {
			var err error
			cls, err = objc.RegisterClass(className, objc.GetClass("NSObject"), nil, nil,
				[]objc.MethodDef{{
					Cmd: objc.RegisterName("askChatGo:userData:error:"),
					Fn:  askChatGo,
				}})
			if err != nil {
				return
			}
		}
		serviceProvider = objc.Send[objc.ID](objc.ID(cls), selNew)
		if serviceProvider == 0 {
			return
		}
		app := objc.Send[objc.ID](objc.ID(objc.GetClass("NSApplication")), selSharedApplication)
		if app == 0 {
			return
		}
		app.Send(selSetServicesProvider, serviceProvider)
		// The Services menu is built from a cache; refresh it so the entry
		// appears without the user logging out.
		updateDynamicServices()
	})
}

// updateDynamicServices asks Launch Services to rescan the services bundles.
func updateDynamicServices() {
	lib, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY)
	if err != nil {
		return
	}
	fn, err := purego.Dlsym(lib, "NSUpdateDynamicServices")
	if err != nil {
		return
	}
	purego.SyscallN(fn)
}

// askChatGo is the IMP behind askChatGo:userData:error:. AppKit hands the
// selected text over in a pasteboard, which is read here and quoted in the
// quick panel's composer.
func askChatGo(self objc.ID, cmd objc.SEL, pboard objc.ID, userData objc.ID, errPtr **objc.ID) {
	text := pasteboardText(pboard)
	if text == "" || serviceTarget == nil {
		return
	}
	serviceTarget.openQuickSelection(text)
}

// pasteboardText reads plain text from a service pasteboard.
func pasteboardText(pboard objc.ID) string {
	if pboard == 0 {
		return ""
	}
	for _, typ := range []string{"public.utf8-plain-text", "public.plain-text", "NSStringPboardType"} {
		if s := nsGoString(objc.Send[objc.ID](pboard, selStringForType, nsString(typ))); s != "" {
			return s
		}
	}
	return ""
}

// nsString makes an autoreleased NSString for selectors that take one.
func nsString(s string) objc.ID {
	if s == "" {
		return 0
	}
	return objc.Send[objc.ID](objc.ID(objc.GetClass("NSString")), selStringWithUTF8, s)
}

// nsGoString copies an NSString into a Go string. The bytes must be copied:
// the string AppKit hands over is autoreleased, and its buffer is gone once
// the service callback returns.
func nsGoString(id objc.ID) string {
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
	return string(unsafe.Slice(p, n))
}
