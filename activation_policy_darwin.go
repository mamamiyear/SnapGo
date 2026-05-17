//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#include <dispatch/dispatch.h>
#import <AppKit/AppKit.h>

static id snipDidHideObserver = nil;

static void snipSetActivationPolicy(bool regular, bool activate) {
    void (^work)(void) = ^{
        [NSApp setActivationPolicy: regular
            ? NSApplicationActivationPolicyRegular
            : NSApplicationActivationPolicyAccessory];
        if (activate) {
            [NSApp unhide:nil];
            [NSApp activateIgnoringOtherApps:YES];
        }
    };

    if ([NSThread isMainThread]) {
        work();
    } else {
        dispatch_sync(dispatch_get_main_queue(), work);
    }
}

static void snipInstallActivationPolicyHooks(void) {
    void (^work)(void) = ^{
        if (snipDidHideObserver != nil) {
            return;
        }
        snipDidHideObserver = [[NSNotificationCenter defaultCenter]
            addObserverForName:NSApplicationDidHideNotification
                        object:NSApp
                         queue:[NSOperationQueue mainQueue]
                    usingBlock:^(__unused NSNotification *note) {
                        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
                    }];
    };

    if ([NSThread isMainThread]) {
        work();
    } else {
        dispatch_sync(dispatch_get_main_queue(), work);
    }
}
*/
import "C"

// installActivationPolicyHooks observes the native hide path used by Wails'
// HideWindowOnClose on macOS. Closing Settings calls `[NSApp hide:nil]`,
// which does not reliably pass through Wails' Go OnBeforeClose hook.
func installActivationPolicyHooks() {
	C.snipInstallActivationPolicyHooks()
}

// showDockIcon makes the app behave like a normal foreground macOS app while
// Settings is visible.
func showDockIcon(activate bool) {
	C.snipSetActivationPolicy(true, C.bool(activate))
}

// hideDockIcon returns the app to menu-bar-agent mode: no persistent Dock
// icon, while the status-bar item remains available.
func hideDockIcon() {
	C.snipSetActivationPolicy(false, false)
}
