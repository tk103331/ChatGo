//go:build darwin

package capture

import (
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// accessibilitySettingsURL opens System Settings on the Accessibility pane.
const accessibilitySettingsURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

type darwinCapture struct{}

func platformCapture() Capture { return darwinCapture{} }

// Permission reports whether the app may read other applications' UI.
func (darwinCapture) Permission() Permission {
	if err := loadNative(); err != nil {
		return Permission{State: PermissionUnsupported}
	}
	if axIsProcessTrusted() {
		return Permission{State: PermissionGranted, SettingsURL: accessibilitySettingsURL}
	}
	return Permission{State: PermissionDenied, SettingsURL: accessibilitySettingsURL}
}

// RequestPermission shows the system's Accessibility prompt, which also adds
// the app to the pane the user would otherwise have to find it in.
func (darwinCapture) RequestPermission() {
	if err := loadNative(); err != nil {
		return
	}
	if axIsProcessTrusted() {
		return
	}
	if axIsProcessTrustedWithOptions == nil || cfDictionaryCreate == nil ||
		kCFTypeDictionaryKeyCallBacks == 0 || kCFTypeDictionaryValueCallBacks == 0 ||
		kCFBooleanTrue == 0 {
		return
	}
	key := cfString("AXTrustedCheckOptionPrompt")
	if key == 0 {
		return
	}
	defer cfRelease(key)
	keys := []uintptr{key}
	values := []uintptr{kCFBooleanTrue}
	dict := cfDictionaryCreate(0, &keys[0], &values[0], 1,
		kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
	if dict == 0 {
		return
	}
	defer cfRelease(dict)
	axIsProcessTrustedWithOptions(dict)
}

// selectionMu serializes the Accessibility reads: they run from the watch
// goroutines and the quick panel alike, and overlapping reads both slow each
// other down and race inside the AX runtime. The staleness check upstream
// keeps the queue short — only the latest event waits its turn.
var selectionMu sync.Mutex

// Selection reads the focused element's selected text.
func (darwinCapture) Selection() (Selection, error) {
	if err := loadNative(); err != nil {
		return Selection{}, ErrUnsupported
	}
	if isSecureEventInputEnabled() {
		return Selection{}, ErrNoSelection
	}
	if !axIsProcessTrusted() {
		return Selection{}, ErrPermission
	}
	var (
		sel Selection
		err error
	)
	selectionMu.Lock()
	defer selectionMu.Unlock()
	withPool(func() { sel, err = readSelection() })
	return sel, err
}

func readSelection() (Selection, error) {
	pid, appName, bundle := frontmostApp()
	debugf("frontmost pid=%d app=%q bundle=%q", pid, appName, bundle)
	if pid == 0 || int(pid) == os.Getpid() {
		return Selection{}, ErrNoSelection
	}
	app := axUIElementCreateApplication(pid)
	if app == 0 {
		return Selection{}, ErrNoSelection
	}
	defer cfRelease(app)
	axUIElementSetMessagingTimeout(app, 0.25)
	// Chromium and Electron build their accessibility tree lazily. This
	// attribute asks them to build it for clients like this one, without
	// which AXSelectedText is often empty in browsers and editors.
	enableManualAccessibility(app)

	focused := copyAttr(app, "AXFocusedUIElement")
	if focused == 0 {
		return Selection{}, ErrNoSelection
	}
	defer cfRelease(focused)
	if attrString(focused, "AXSubrole") == "AXSecureTextField" {
		return Selection{}, ErrNoSelection
	}
	text := strings.TrimRight(attrString(focused, "AXSelectedText"), "\x00")
	if strings.TrimSpace(text) == "" {
		// Some applications expose the selected range but not the selected
		// text attribute; ask them for the string in that range instead.
		text = strings.TrimRight(selectedRangeText(focused), "\x00")
	}
	if !AcceptText(text, 1) {
		debugf("no usable selection in %q", appName)
		return Selection{}, ErrNoSelection
	}
	rect := selectionRect(focused)
	debugf("selection from %q: %d runes, rect=%+v", appName, utf8.RuneCountInString(text), rect)
	return Selection{
		Text:     text,
		Rect:     rect,
		App:      appName,
		BundleID: bundle,
		PID:      int(pid),
		Source:   SourceAX,
	}, nil
}

// selectedRangeText reads the selected text through AXStringForRange, for
// elements that answer the range but leave AXSelectedText empty.
func selectedRangeText(el uintptr) string {
	rng := copyAttr(el, "AXSelectedTextRange")
	if rng == 0 {
		return ""
	}
	defer cfRelease(rng)
	key := cfString("AXStringForRange")
	if key == 0 {
		return ""
	}
	defer cfRelease(key)
	var out uintptr
	if axUIElementCopyParameterizedAttribute(el, key, rng, &out) != 0 || out == 0 {
		return ""
	}
	defer cfRelease(out)
	return cfStringToGo(out)
}

// selectionRect is the on-screen box of the selection: AXBoundsForRange when
// the element answers it, the element's own frame otherwise.
func selectionRect(el uintptr) Rect {
	if rng := copyAttr(el, "AXSelectedTextRange"); rng != 0 {
		defer cfRelease(rng)
		if key := cfString("AXBoundsForRange"); key != 0 {
			defer cfRelease(key)
			var out uintptr
			if axUIElementCopyParameterizedAttribute(el, key, rng, &out) == 0 && out != 0 {
				var r cgRect
				ok := cfGetTypeID(out) == axValueGetTypeID() &&
					axValueGetValue(out, axValueCGRectType, unsafe.Pointer(&r))
				cfRelease(out)
				if ok {
					return rectOf(r)
				}
			}
		}
	}
	p, okPoint := attrPoint(el, "AXPosition")
	s, okSize := attrSize(el, "AXSize")
	if okPoint && okSize {
		return rectOf(cgRect{Origin: p, Size: s})
	}
	return Rect{}
}

func rectOf(r cgRect) Rect {
	return Rect{
		X:      int(math.Round(r.Origin.X)),
		Y:      int(math.Round(r.Origin.Y)),
		Width:  int(math.Round(r.Size.Width)),
		Height: int(math.Round(r.Size.Height)),
	}
}

// enableManualAccessibility asks a Chromium-based app to expose its
// accessibility tree. Apps that do not know the attribute ignore it.
func enableManualAccessibility(app uintptr) {
	if kCFBooleanTrue == 0 {
		return
	}
	key := cfString("AXManualAccessibility")
	if key == 0 {
		return
	}
	defer cfRelease(key)
	axUIElementSetAttributeValue(app, key, kCFBooleanTrue)
}

// Directory prefers a file URL on the clipboard, then Finder's selection,
// then a path copied as plain text.
func (c darwinCapture) Directory() (string, error) {
	if err := loadNative(); err != nil {
		return "", ErrUnsupported
	}
	dir := ""
	withPool(func() {
		if p := pasteboardString("public.file-url"); p != "" {
			if path, ok := ParseFileURL(p); ok && ExistingDir(path) {
				dir = path
				return
			}
		}
		if axIsProcessTrusted() {
			if path := finderSelection(); path != "" && ExistingDir(path) {
				dir = path
				return
			}
		}
		if p := pasteboardString("public.utf8-plain-text"); p != "" {
			if path, ok := ParseFileURL(p); ok && ExistingDir(path) {
				dir = path
			}
		}
	})
	if dir == "" {
		debugf("no directory on the clipboard or in Finder")
		return "", ErrNoDirectory
	}
	debugf("directory %q", dir)
	return dir, nil
}

// ClipboardText is the fallback when Accessibility is not granted.
func (darwinCapture) ClipboardText() string {
	if err := loadNative(); err != nil {
		return ""
	}
	text := ""
	withPool(func() { text = pasteboardString("public.utf8-plain-text") })
	return strings.TrimRight(text, "\x00")
}

// CopySelection is not used: the Accessibility read is the selection API on
// macOS.
func (darwinCapture) CopySelection() (string, bool) { return "", false }

// finderSelection is the folder the user selected in Finder, read from the
// selected row's AXURL. It walks a bounded part of the window's tree: Finder
// nests the browser several levels deep, and the tree can be large.
func finderSelection() string {
	pid, _, bundle := frontmostApp()
	if pid == 0 || bundle != "com.apple.finder" {
		return ""
	}
	app := axUIElementCreateApplication(pid)
	if app == 0 {
		return ""
	}
	defer cfRelease(app)
	axUIElementSetMessagingTimeout(app, 0.25)
	win := copyAttr(app, "AXFocusedWindow")
	if win == 0 {
		win = copyAttr(app, "AXMainWindow")
	}
	if win == 0 {
		return ""
	}
	defer cfRelease(win)
	visited := 0
	return findSelectedURL(win, 0, &visited)
}

func findSelectedURL(el uintptr, depth int, visited *int) string {
	if depth > 8 || *visited > 500 {
		return ""
	}
	*visited = *visited + 1

	if sel := copyAttr(el, "AXSelectedChildren"); sel != 0 {
		n := cfArrayGetCount(sel)
		if n > 0 {
			url := attrURL(cfArrayGetValueAtIndex(sel, 0))
			cfRelease(sel)
			if path, ok := ParseFileURL(url); ok {
				return path
			}
		} else {
			cfRelease(sel)
		}
	}

	kids := copyAttr(el, "AXChildren")
	if kids == 0 {
		return ""
	}
	defer cfRelease(kids)
	n := cfArrayGetCount(kids)
	if n > 200 {
		n = 200
	}
	for i := int64(0); i < n; i++ {
		if path := findSelectedURL(cfArrayGetValueAtIndex(kids, i), depth+1, visited); path != "" {
			return path
		}
	}
	return ""
}

// WatchDismiss calls fn when the user presses a mouse button outside this
// application. The monitor itself needs no permission, and it never sees
// events sent to this app, so a click on the bar is not a dismissal. Call it
// on the main thread.
func (darwinCapture) WatchDismiss(fn func()) (stop func()) {
	if err := loadNative(); err != nil {
		return func() {}
	}
	block := objc.NewBlock(func(_ objc.Block, _ objc.ID) { fn() })
	// NSEventMaskLeftMouseDown | NSEventMaskRightMouseDown.
	const mouseDownMask = uint64(1<<1 | 1<<3)
	monitor := objc.Send[objc.ID](objc.ID(objc.GetClass("NSEvent")), selAddGlobalMonitor, mouseDownMask, block)
	if monitor == 0 {
		block.Release()
		return func() {}
	}
	return func() {
		objc.Send[objc.ID](objc.ID(objc.GetClass("NSEvent")), selRemoveMonitor, monitor)
		block.Release()
	}
}

// Watch calls fn after a mouse selection ends in another app. The monitor
// itself needs no permission (only key events do); the AX read inside does,
// which is why Watch installs nothing until Accessibility is granted. Call it
// on the main thread.
func (d darwinCapture) Watch(fn func(Selection)) (stop func()) {
	if err := loadNative(); err != nil {
		return func() {}
	}
	if !axIsProcessTrusted() {
		return func() {}
	}

	var mu sync.Mutex
	stopped := false
	var gen uint64

	handler := func(_ objc.Block, _ objc.ID) {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return
		}
		gen++
		mine := gen
		mu.Unlock()
		go func() {
			// Applications settle the selection just after the mouse comes
			// up; reading at once reads the previous one.
			time.Sleep(90 * time.Millisecond)
			mu.Lock()
			stale := stopped || mine != gen
			mu.Unlock()
			if stale {
				return
			}
			sel, err := d.Selection()
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if stopped {
				return
			}
			fn(sel)
		}()
	}

	block := objc.NewBlock(handler)
	// NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp.
	const mouseUpMask = uint64(1<<2 | 1<<4)
	monitor := objc.Send[objc.ID](objc.ID(objc.GetClass("NSEvent")), selAddGlobalMonitor, mouseUpMask, block)
	if monitor == 0 {
		block.Release()
		return func() {}
	}
	return func() {
		mu.Lock()
		stopped = true
		mu.Unlock()
		objc.Send[objc.ID](objc.ID(objc.GetClass("NSEvent")), selRemoveMonitor, monitor)
		block.Release()
	}
}
