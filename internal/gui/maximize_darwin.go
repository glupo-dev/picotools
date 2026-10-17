package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// Resizes the window to the screen area not covered by the menu bar and the Dock.
static void maximizeNSWindow(uintptr_t ptr) {
	NSWindow *w = (NSWindow *)ptr;
	dispatch_async(dispatch_get_main_queue(), ^{
		NSScreen *s = [w screen] ? [w screen] : [NSScreen mainScreen];
		[w setFrame:[s visibleFrame] display:YES animate:NO];
	});
}
*/
import "C"

import "fyne.io/fyne/v2/driver"

func maximizeNative(ctx any) bool {
	c, ok := ctx.(driver.MacWindowContext)
	if !ok || c.NSWindow == 0 {
		return false
	}
	C.maximizeNSWindow(C.uintptr_t(c.NSWindow))
	return true
}
