//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics
#include <math.h>
#include <dispatch/dispatch.h>
#import <AppKit/AppKit.h>

extern void nativeOverlayConfirm(int x, int y, int w, int h);
extern void nativeOverlayCancel(void);

static NSWindow *nativeOverlayWindow = nil;
static id nativeOverlayKeyMonitor = nil;

@interface SnipNativeOverlayPanel : NSPanel
@end

@implementation SnipNativeOverlayPanel
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return YES; }
@end

@interface SnipNativeOverlayView : NSView
@property BOOL hasSelection;
@property BOOL creating;
@property BOOL moving;
@property NSRect selection;
@property NSPoint anchor;
@property NSPoint moveOffset;
@property(strong) NSButton *cancelButton;
@property(strong) NSButton *uploadButton;
@property(strong) NSTextField *sizeLabel;
@property(strong) NSTextField *hintLabel;
- (void)syncControls;
- (void)styleControls;
- (void)confirmSelection;
- (void)cancelSelection;
@end

@implementation SnipNativeOverlayView
- (BOOL)isFlipped { return YES; }
- (BOOL)acceptsFirstResponder { return YES; }

- (instancetype)initWithFrame:(NSRect)frame {
    self = [super initWithFrame:frame];
    if (self) {
        [self setWantsLayer:YES];
        [[self layer] setBackgroundColor:[[NSColor clearColor] CGColor]];

        _cancelButton = [NSButton buttonWithTitle:@"Cancel" target:self action:@selector(cancelSelection)];
        _uploadButton = [NSButton buttonWithTitle:@"Upload & copy" target:self action:@selector(confirmSelection)];
        _sizeLabel = [NSTextField labelWithString:@""];
        _hintLabel = [NSTextField labelWithString:@"Drag to select an area  ·  Esc to cancel"];
        [self styleControls];

        for (NSView *view in @[_cancelButton, _uploadButton, _sizeLabel, _hintLabel]) {
            [self addSubview:view];
        }
        [_cancelButton setHidden:YES];
        [_uploadButton setHidden:YES];
        [_sizeLabel setHidden:YES];

        [_sizeLabel setTextColor:[NSColor whiteColor]];
        [_sizeLabel setBackgroundColor:[NSColor colorWithCalibratedWhite:0.08 alpha:0.78]];
        [_sizeLabel setDrawsBackground:YES];
        [_sizeLabel setBezeled:NO];
        [_sizeLabel setAlignment:NSTextAlignmentCenter];

        [_hintLabel setTextColor:[NSColor whiteColor]];
        [_hintLabel setBackgroundColor:[NSColor colorWithCalibratedWhite:0.0 alpha:0.5]];
        [_hintLabel setDrawsBackground:YES];
        [_hintLabel setBezeled:NO];
        [_hintLabel setAlignment:NSTextAlignmentCenter];
        [_hintLabel setFrame:NSMakeRect((frame.size.width - 320) / 2, 24, 320, 24)];
    }
    return self;
}

- (void)styleButton:(NSButton *)button background:(NSColor *)background foreground:(NSColor *)foreground {
    [button setBordered:NO];
    [button setBezelStyle:NSBezelStyleRegularSquare];
    [button setWantsLayer:YES];
    [[button layer] setCornerRadius:5];
    [[button layer] setBackgroundColor:[background CGColor]];
    NSMutableAttributedString *title = [[NSMutableAttributedString alloc] initWithString:[button title]];
    [title addAttribute:NSForegroundColorAttributeName value:foreground range:NSMakeRange(0, [title length])];
    [title addAttribute:NSFontAttributeName value:[NSFont systemFontOfSize:12 weight:NSFontWeightMedium] range:NSMakeRange(0, [title length])];
    [button setAttributedTitle:title];
}

- (void)styleControls {
    [self styleButton:_cancelButton
           background:[NSColor colorWithCalibratedWhite:0.12 alpha:0.96]
           foreground:[NSColor colorWithCalibratedWhite:0.86 alpha:1.0]];
    [self styleButton:_uploadButton
           background:[NSColor colorWithCalibratedRed:59.0/255.0 green:130.0/255.0 blue:246.0/255.0 alpha:1.0]
           foreground:[NSColor whiteColor]];
}

- (void)drawRect:(NSRect)dirtyRect {
    [[NSColor colorWithCalibratedWhite:0.0 alpha:0.48] setFill];
    if (_hasSelection) {
        NSRect top = NSMakeRect(0, 0, self.bounds.size.width, _selection.origin.y);
        NSRect left = NSMakeRect(0, _selection.origin.y, _selection.origin.x, _selection.size.height);
        NSRect right = NSMakeRect(NSMaxX(_selection), _selection.origin.y, self.bounds.size.width - NSMaxX(_selection), _selection.size.height);
        NSRect bottom = NSMakeRect(0, NSMaxY(_selection), self.bounds.size.width, self.bounds.size.height - NSMaxY(_selection));
        NSRectFill(top);
        NSRectFill(left);
        NSRectFill(right);
        NSRectFill(bottom);

        NSBezierPath *border = [NSBezierPath bezierPathWithRect:NSInsetRect(_selection, 0.75, 0.75)];
        [border setLineWidth:1.5];
        [[NSColor colorWithCalibratedRed:59.0/255.0 green:130.0/255.0 blue:246.0/255.0 alpha:1.0] setStroke];
        [border stroke];

        CGFloat corner = 8;
        NSBezierPath *corners = [NSBezierPath bezierPath];
        [corners moveToPoint:NSMakePoint(NSMinX(_selection), NSMinY(_selection) + corner)];
        [corners lineToPoint:NSMakePoint(NSMinX(_selection), NSMinY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMinX(_selection) + corner, NSMinY(_selection))];
        [corners moveToPoint:NSMakePoint(NSMaxX(_selection) - corner, NSMinY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMaxX(_selection), NSMinY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMaxX(_selection), NSMinY(_selection) + corner)];
        [corners moveToPoint:NSMakePoint(NSMaxX(_selection), NSMaxY(_selection) - corner)];
        [corners lineToPoint:NSMakePoint(NSMaxX(_selection), NSMaxY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMaxX(_selection) - corner, NSMaxY(_selection))];
        [corners moveToPoint:NSMakePoint(NSMinX(_selection) + corner, NSMaxY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMinX(_selection), NSMaxY(_selection))];
        [corners lineToPoint:NSMakePoint(NSMinX(_selection), NSMaxY(_selection) - corner)];
        [corners setLineWidth:3];
        [corners stroke];
    } else {
        NSRectFill(self.bounds);
    }
}

- (void)mouseDown:(NSEvent *)event {
    NSPoint p = [self convertPoint:[event locationInWindow] fromView:nil];
    if (_hasSelection && NSPointInRect(p, _selection)) {
        _moving = YES;
        _creating = NO;
        _moveOffset = NSMakePoint(p.x - _selection.origin.x, p.y - _selection.origin.y);
    } else {
        _creating = YES;
        _moving = NO;
        _hasSelection = YES;
        _anchor = p;
        _selection = NSMakeRect(p.x, p.y, 0, 0);
    }
    [self syncControls];
}

- (void)mouseDragged:(NSEvent *)event {
    NSPoint p = [self convertPoint:[event locationInWindow] fromView:nil];
    if (_creating) {
        CGFloat x = MIN(_anchor.x, p.x);
        CGFloat y = MIN(_anchor.y, p.y);
        _selection = NSMakeRect(x, y, fabs(p.x - _anchor.x), fabs(p.y - _anchor.y));
    } else if (_moving) {
        CGFloat x = p.x - _moveOffset.x;
        CGFloat y = p.y - _moveOffset.y;
        x = MAX(0, MIN(x, self.bounds.size.width - _selection.size.width));
        y = MAX(0, MIN(y, self.bounds.size.height - _selection.size.height));
        _selection.origin = NSMakePoint(x, y);
    }
    [self syncControls];
    [self setNeedsDisplay:YES];
}

- (void)mouseUp:(NSEvent *)event {
    _creating = NO;
    _moving = NO;
    if (_hasSelection && (_selection.size.width < 4 || _selection.size.height < 4)) {
        _hasSelection = NO;
    }
    [self syncControls];
    [self setNeedsDisplay:YES];
}

- (void)keyDown:(NSEvent *)event {
    if ([event keyCode] == 53 || [[event charactersIgnoringModifiers] isEqualToString:@"\033"]) {
        [self cancelSelection];
    } else if (([event keyCode] == 36 || [[event charactersIgnoringModifiers] isEqualToString:@"\r"]) && _hasSelection) {
        [self confirmSelection];
    } else {
        [super keyDown:event];
    }
}

- (void)rightMouseDown:(NSEvent *)event {
    [self cancelSelection];
}

- (void)syncControls {
    BOOL visible = _hasSelection && _selection.size.width >= 4 && _selection.size.height >= 4;
    [_cancelButton setHidden:!visible];
    [_uploadButton setHidden:!visible];
    [_sizeLabel setHidden:!visible];
    [_hintLabel setHidden:_hasSelection];

    if (!visible) {
        return;
    }

    [_sizeLabel setStringValue:[NSString stringWithFormat:@"%.0f × %.0f", _selection.size.width, _selection.size.height]];
    [_sizeLabel setFrame:NSMakeRect(_selection.origin.x, MAX(0, _selection.origin.y - 26), 110, 22)];

    CGFloat toolbarW = 220;
    CGFloat toolbarH = 40;
    CGFloat x = _selection.origin.x + _selection.size.width - toolbarW;
    CGFloat y = _selection.origin.y + _selection.size.height + 8;
    if (y + toolbarH > self.bounds.size.height) {
        y = _selection.origin.y + _selection.size.height - toolbarH - 8;
    }
    x = MAX(0, MIN(x, self.bounds.size.width - toolbarW));

    [_cancelButton setFrame:NSMakeRect(x, y, 86, 32)];
    [_uploadButton setFrame:NSMakeRect(x + 92, y, 128, 32)];
}

- (void)confirmSelection {
    if (!_hasSelection) {
        return;
    }
    NSRect r = _selection;
    [nativeOverlayWindow orderOut:nil];
        if (nativeOverlayKeyMonitor != nil) {
            [NSEvent removeMonitor:nativeOverlayKeyMonitor];
            nativeOverlayKeyMonitor = nil;
        }
    nativeOverlayWindow = nil;
    nativeOverlayConfirm((int)llround(r.origin.x), (int)llround(r.origin.y), (int)llround(r.size.width), (int)llround(r.size.height));
}

- (void)cancelSelection {
    [nativeOverlayWindow orderOut:nil];
    if (nativeOverlayKeyMonitor != nil) {
        [NSEvent removeMonitor:nativeOverlayKeyMonitor];
        nativeOverlayKeyMonitor = nil;
    }
    nativeOverlayWindow = nil;
    nativeOverlayCancel();
}
@end

static void snipShowNativeOverlay(int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSScreen *screen = [NSScreen mainScreen];
        if (screen == nil) {
            nativeOverlayCancel();
            return;
        }

        NSRect frame = [screen frame];
        [NSApp activateIgnoringOtherApps:YES];

        nativeOverlayWindow = [[SnipNativeOverlayPanel alloc]
            initWithContentRect:frame
                      styleMask:NSWindowStyleMaskBorderless
                        backing:NSBackingStoreBuffered
                          defer:NO];
        [nativeOverlayWindow setOpaque:NO];
        [nativeOverlayWindow setBackgroundColor:[NSColor clearColor]];
        [nativeOverlayWindow setLevel:NSScreenSaverWindowLevel];
        [nativeOverlayWindow setHidesOnDeactivate:NO];
        [(NSPanel *)nativeOverlayWindow setFloatingPanel:YES];
        [nativeOverlayWindow setCollectionBehavior:
            NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorFullScreenAuxiliary |
            NSWindowCollectionBehaviorStationary];
        [nativeOverlayWindow setIgnoresMouseEvents:NO];

        SnipNativeOverlayView *view = [[SnipNativeOverlayView alloc]
            initWithFrame:NSMakeRect(0, 0, frame.size.width, frame.size.height)];
        [nativeOverlayWindow setContentView:view];
        [nativeOverlayWindow makeKeyAndOrderFront:nil];
        [nativeOverlayWindow makeFirstResponder:view];

        nativeOverlayKeyMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *event) {
            if ([event keyCode] == 53) {
                [view cancelSelection];
                return nil;
            }
            if ([event keyCode] == 36 && [view hasSelection]) {
                [view confirmSelection];
                return nil;
            }
            return event;
        }];
    });
}
*/
import "C"
import (
	"sync"

	"github.com/mmmy/snapgo/internal/infrastructure/display"
)

var nativeOverlayState struct {
	sync.Mutex
	app *App
}

// showNativeCaptureOverlay uses a macOS-native transparent panel instead of
// the Wails WebView overlay, avoiding WKWebView's opaque grey backing layer.
func showNativeCaptureOverlay(app *App, info display.Info) bool {
	nativeOverlayState.Lock()
	nativeOverlayState.app = app
	nativeOverlayState.Unlock()
	C.snipShowNativeOverlay(C.int(info.CSSWidth), C.int(info.CSSHeight))
	return true
}
