//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include <dispatch/dispatch.h>
#import <AppKit/AppKit.h>

static NSWindow* snipMainWindow(void) {
    NSWindow *window = [NSApp mainWindow];
    if (window == nil && [[NSApp windows] count] > 0) {
        window = [[NSApp windows] objectAtIndex:0];
    }
    return window;
}

// snipMakeViewTransparent recursively clears AppKit/WKWebView backing
// surfaces. Setting only NSWindow transparent is insufficient for Wails:
// WKWebView may still paint an opaque default grey background.
static void snipMakeViewTransparent(NSView *view) {
    if (view == nil) {
        return;
    }

    Class wkWebViewClass = NSClassFromString(@"WKWebView");
    if (wkWebViewClass != nil && [view isKindOfClass:wkWebViewClass]) {
        @try {
            [view setValue:@NO forKey:@"drawsBackground"];
        } @catch (__unused NSException *exception) {
        }
        @try {
            id scrollView = [view valueForKey:@"scrollView"];
            if ([scrollView isKindOfClass:[NSScrollView class]]) {
                [(NSScrollView *)scrollView setDrawsBackground:NO];
                [(NSScrollView *)scrollView setBackgroundColor:[NSColor clearColor]];
            }
        } @catch (__unused NSException *exception) {
        }
    }

    if ([view isKindOfClass:[NSScrollView class]]) {
        NSScrollView *scrollView = (NSScrollView *)view;
        [scrollView setDrawsBackground:NO];
        [scrollView setBackgroundColor:[NSColor clearColor]];
    }

    [view setWantsLayer:YES];
    if ([view layer] != nil) {
        [[view layer] setOpaque:NO];
        [[view layer] setBackgroundColor:[[NSColor clearColor] CGColor]];
    }

    @try {
        [view setValue:@NO forKey:@"opaque"];
    } @catch (__unused NSException *exception) {
    }

    @try {
        [view setValue:@NO forKey:@"drawsBackground"];
    } @catch (__unused NSException *exception) {
    }

    for (NSView *subview in [view subviews]) {
        snipMakeViewTransparent(subview);
    }
}

static void snipConfigureOverlayWindow(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = snipMainWindow();
        NSScreen *screen = [NSScreen mainScreen];
        if (window == nil || screen == nil) {
            return;
        }

        [window setOpaque:NO];
        [window setBackgroundColor:[NSColor clearColor]];
        snipMakeViewTransparent([window contentView]);
        [window setLevel:NSScreenSaverWindowLevel];
        [window setCollectionBehavior:
            NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorFullScreenAuxiliary |
            NSWindowCollectionBehaviorStationary];
        [window setFrame:[screen frame] display:YES];
    });
}

static void snipRestoreOverlayWindow(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = snipMainWindow();
        if (window == nil) {
            return;
        }

        [window setLevel:NSNormalWindowLevel];
        [window setCollectionBehavior:NSWindowCollectionBehaviorDefault];
        [window setOpaque:YES];
        [window setBackgroundColor:
            [NSColor colorWithCalibratedRed:246.0/255.0
                                      green:247.0/255.0
                                       blue:250.0/255.0
                                      alpha:1.0]];
    });
}
*/
import "C"

// configureOverlayWindow promotes the Wails NSWindow to a transparent,
// screen-level overlay. Wails can size a frameless window, but AppKit-only
// flags are needed to cover the menu bar and keep the WebView transparent.
func configureOverlayWindow() {
	C.snipConfigureOverlayWindow()
}

// restoreOverlayWindow returns the NSWindow to normal app-window behaviour
// before the settings UI is shown again.
func restoreOverlayWindow() {
	C.snipRestoreOverlayWindow()
}
