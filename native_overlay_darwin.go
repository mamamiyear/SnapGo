//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework AppKit -framework CoreGraphics -framework QuartzCore
#include <math.h>
#include <dispatch/dispatch.h>
#import <objc/runtime.h>
#import <AppKit/AppKit.h>
#import <QuartzCore/QuartzCore.h>

extern void nativeOverlayConfirm(int x, int y, int w, int h, const char *annotationsJSON);
extern void nativeOverlayCopy(int x, int y, int w, int h, const char *annotationsJSON);
extern void nativeOverlaySave(int x, int y, int w, int h, const char *annotationsJSON, const char *dir);
extern void nativeOverlaySaveRemote(int x, int y, int w, int h, const char *annotationsJSON);
extern void nativeOverlayCancel(void);

static NSWindow *nativeOverlayWindow = nil;
static id nativeOverlayKeyMonitor = nil;

@interface SnipNativeOverlayPanel : NSPanel
@end

@implementation SnipNativeOverlayPanel
- (BOOL)canBecomeKeyWindow { return YES; }
- (BOOL)canBecomeMainWindow { return YES; }
@end

// SnipHoverButton — NSButton subclass that animates its tint color on hover.
//
// Design rationale:
// - NSButton has no built-in hover state; we install an NSTrackingArea that
//   tracks mouseEntered/mouseExited regardless of focus state.
// - When the cursor enters, the content tint color animates from `baseColor`
//   to `hoverColor` over 0.2s using NSAnimationContext, matching the spec.
//   When the cursor leaves, the color reverts immediately (no animation).
// - `baseColor`/`hoverColor` are configurable per-instance so each action
//   button can keep a shared white default but expose its own hover hue
//   (cancel = red, others = blue).
@interface SnipHoverButton : NSButton
@property(strong) NSColor *baseColor;
@property(strong) NSColor *hoverColor;
@property(strong) NSTrackingArea *hoverTrackingArea;
@end

@implementation SnipHoverButton
- (void)updateTrackingAreas {
    [super updateTrackingAreas];
    if (_hoverTrackingArea) {
        [self removeTrackingArea:_hoverTrackingArea];
    }
    NSTrackingAreaOptions opts = NSTrackingMouseEnteredAndExited |
                                 NSTrackingActiveAlways |
                                 NSTrackingInVisibleRect;
    _hoverTrackingArea = [[NSTrackingArea alloc] initWithRect:NSZeroRect
                                                      options:opts
                                                        owner:self
                                                     userInfo:nil];
    [self addTrackingArea:_hoverTrackingArea];
}
- (void)mouseEntered:(NSEvent *)event {
    if (@available(macOS 10.14, *)) {
        NSColor *target = _hoverColor ?: [NSColor whiteColor];
        // Animate the tint color transition over 0.2s for a soft fade-in.
        [NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
            [ctx setDuration:0.2];
            [ctx setTimingFunction:[CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseInEaseOut]];
            [[self animator] setContentTintColor:target];
        } completionHandler:nil];
    }
}
- (void)mouseExited:(NSEvent *)event {
    if (@available(macOS 10.14, *)) {
        NSColor *base = _baseColor ?: [NSColor whiteColor];
        // Snap back to the base color instantly (no animation) per spec.
        [self setContentTintColor:base];
    }
}
@end

@interface SnipNativeOverlayView : NSView
@property BOOL hasSelection;
@property BOOL creating;
@property BOOL moving;
@property BOOL resizing;
@property BOOL annotating;
@property NSRect selection;
@property NSPoint anchor;
@property NSPoint moveOffset;
@property NSRect resizeStart;
@property NSString *resizeHandle;
@property NSString *activeTool;
@property(strong) NSColor *activeColor;
@property(strong) NSMutableArray<NSDictionary *> *annotations;
@property(strong) NSMutableDictionary *draftAnnotation;
@property(strong) NSButton *cancelButton;
// Use `clipboardButton` (not `copyButton`) to avoid ARC's "copy" method
// family ownership rule which would otherwise treat the synthesized getter
// as returning a +1 retained object.
@property(strong) NSButton *clipboardButton;
@property(strong) NSButton *saveButton;
// `saveRemoteButton` triggers a placeholder action ("not yet supported")
// per product spec; the icon comes from save-remote.svg.
@property(strong) NSButton *saveRemoteButton;
@property(strong) NSButton *uploadButton;
@property(strong) NSButton *penButton;
@property(strong) NSButton *rectButton;
@property(strong) NSButton *ellipseButton;
@property(strong) NSButton *colorButton;
@property(strong) NSButton *undoButton;
@property(strong) NSView *paletteView;
@property(strong) NSView *actionToolbarBg;
@property(strong) NSTextField *sizeLabel;
@property(strong) NSTextField *hintLabel;
- (void)syncControls;
- (void)styleControls;
- (void)confirmSelection;
- (void)copySelection;
- (void)saveSelection;
- (void)saveRemoteSelection;
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

        _activeTool = nil;
        _activeColor = [NSColor colorWithCalibratedRed:239.0/255.0 green:68.0/255.0 blue:68.0/255.0 alpha:1.0];
        _annotations = [NSMutableArray array];

        // Use SnipHoverButton for the 5 action buttons so we get hover-color
        // animation. Annotation buttons stay as plain NSButton because they
        // already have explicit on/off "active" styling.
        _cancelButton = [SnipHoverButton buttonWithTitle:@"" target:self action:@selector(cancelSelection)];
        _clipboardButton = [SnipHoverButton buttonWithTitle:@"" target:self action:@selector(copySelection)];
        _saveButton = [SnipHoverButton buttonWithTitle:@"" target:self action:@selector(saveSelection)];
        _saveRemoteButton = [SnipHoverButton buttonWithTitle:@"" target:self action:@selector(saveRemoteSelection)];
        _uploadButton = [SnipHoverButton buttonWithTitle:@"" target:self action:@selector(confirmSelection)];
        // Tooltip strings shown on hover for each action button.
        // Localized in Chinese to match the rest of the action UI surface.
        [_cancelButton setToolTip:@"取消截图"];
        [_clipboardButton setToolTip:@"复制图片"];
        [_saveButton setToolTip:@"保存本地"];
        [_saveRemoteButton setToolTip:@"保存远端"];
        [_uploadButton setToolTip:@"上传云端"];
        _penButton = [NSButton buttonWithTitle:@"" target:self action:@selector(selectPen)];
        _rectButton = [NSButton buttonWithTitle:@"" target:self action:@selector(selectRect)];
        _ellipseButton = [NSButton buttonWithTitle:@"" target:self action:@selector(selectEllipse)];
        _colorButton = [NSButton buttonWithTitle:@"" target:self action:@selector(togglePalette)];
        _undoButton = [NSButton buttonWithTitle:@"" target:self action:@selector(undoAnnotation)];
        _paletteView = [[NSView alloc] initWithFrame:NSZeroRect];
        // Dark rounded background container behind the action icon buttons,
        // mirroring the look of `.action-toolbar` in the Vue overlay.
        _actionToolbarBg = [[NSView alloc] initWithFrame:NSZeroRect];
        [_actionToolbarBg setWantsLayer:YES];
        [[_actionToolbarBg layer] setCornerRadius:8];
        [[_actionToolbarBg layer] setBackgroundColor:[[NSColor colorWithCalibratedWhite:0.11 alpha:0.94] CGColor]];
        _sizeLabel = [NSTextField labelWithString:@""];
        _hintLabel = [NSTextField labelWithString:@"Drag to select an area  ·  Esc to cancel"];
        [self styleControls];

        // Add the action toolbar background BEFORE the buttons so it sits
        // behind them in the view hierarchy (AppKit z-order = subview order).
        [self addSubview:_actionToolbarBg];
        for (NSView *view in @[_cancelButton, _clipboardButton, _saveButton, _saveRemoteButton, _uploadButton, _penButton, _rectButton, _ellipseButton, _colorButton, _undoButton, _paletteView, _sizeLabel, _hintLabel]) {
            [self addSubview:view];
        }
        for (NSView *view in @[_cancelButton, _clipboardButton, _saveButton, _saveRemoteButton, _uploadButton, _penButton, _rectButton, _ellipseButton, _colorButton, _undoButton, _paletteView, _actionToolbarBg]) {
            [view setHidden:YES];
        }
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

- (NSString *)hexForColor:(NSColor *)color {
    NSColor *rgb = [color colorUsingColorSpace:[NSColorSpace sRGBColorSpace]];
    NSInteger r = (NSInteger)llround([rgb redComponent] * 255.0);
    NSInteger g = (NSInteger)llround([rgb greenComponent] * 255.0);
    NSInteger b = (NSInteger)llround([rgb blueComponent] * 255.0);
    return [NSString stringWithFormat:@"#%02lx%02lx%02lx", (long)r, (long)g, (long)b];
}

- (NSImage *)iconFromSVG:(NSString *)svg {
    NSData *data = [svg dataUsingEncoding:NSUTF8StringEncoding];
    NSImage *image = [[NSImage alloc] initWithData:data];
    [image setTemplate:YES];
    return image;
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

- (void)styleIconButton:(NSButton *)button image:(NSImage *)image {
    [button setBordered:NO];
    [button setBezelStyle:NSBezelStyleRegularSquare];
    [button setImagePosition:NSImageOnly];
    [button setImageScaling:NSImageScaleProportionallyDown];
    if (@available(macOS 10.14, *)) {
        [button setContentTintColor:[NSColor whiteColor]];
    }
    [button setImage:image];
    [button setWantsLayer:YES];
    [[button layer] setCornerRadius:5];
    [[button layer] setBackgroundColor:[[NSColor clearColor] CGColor]];
}

- (void)styleControls {
    // Cancel / Copy / Save / Save-remote / Upload — square icon buttons
    // rendered from the user-supplied SVG assets. All five default to white
    // and animate to a per-action accent color on hover (cancel = red,
    // others = blue). The base "white default" replaces the previous blue
    // upload tint so the toolbar reads as a uniform icon row.
    NSImage *cancelIcon = [self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M512 85.333333c235.648 0 426.666667 191.018667 426.666667 426.666667s-191.018667 426.666667-426.666667 426.666667S85.333333 747.648 85.333333 512 276.352 85.333333 512 85.333333zM170.666667 512a341.333333 341.333333 0 0 0 550.613333 269.653333L242.304 302.762667A339.84 339.84 0 0 0 170.666667 512z m341.333333-341.333333c-78.848 0-151.466667 26.752-209.28 71.68l478.976 478.933333A341.333333 341.333333 0 0 0 512 170.666667z' fill='white'/></svg>"];
    NSImage *copyIcon = [self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M768 256a85.333333 85.333333 0 0 1 85.333333 85.333333v512a85.333333 85.333333 0 0 1-85.333333 85.333334h-341.333333a85.333333 85.333333 0 0 1-85.333334-85.333334V341.333333a85.333333 85.333333 0 0 1 85.333334-85.333333h341.333333z m0 85.333333h-341.333333v512h341.333333V341.333333z m-128-256a42.666667 42.666667 0 0 1 42.666667 42.666667l-0.042667 42.666667H256l-0.042667 597.333333H213.333333a42.666667 42.666667 0 0 1-42.666666-42.666667V170.666667a85.333333 85.333333 0 0 1 85.333333-85.333334h384z' fill='white'/></svg>"];
    NSImage *saveIcon = [self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M896 810.666667a85.333333 85.333333 0 0 1-85.333333 85.333333H213.333333a85.333333 85.333333 0 0 1-85.333333-85.333333V213.333333a85.333333 85.333333 0 0 1 85.333333-85.333333h250.368a85.333333 85.333333 0 0 1 73.173334 41.429333L563.2 213.333333H810.666667a85.333333 85.333333 0 0 1 85.333333 85.333334v512z m-85.333333-341.333334H213.333333v341.333334h597.333334v-341.333334z m-346.965334-256H213.333333v170.666667h597.333334V298.666667h-271.616a42.666667 42.666667 0 0 1-36.608-20.736L463.701333 213.333333z' fill='white'/></svg>"];
    // save-remote.svg — provided by the user; its content is the same arrow
    // icon as the previous upload, so we reuse it verbatim here.
    NSImage *saveRemoteIcon = [self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M481.749333 353.792c16.682667-16.64 43.690667-16.64 60.373334 0l120.917333 120.832 3.541333 4.010667a42.666667 42.666667 0 0 1-63.872 56.32L554.666667 486.954667V896a42.666667 42.666667 0 0 1-85.333334 0v-409.173333l-48.170666 48.128a42.666667 42.666667 0 0 1-60.330667 0l-3.541333-4.010667a42.666667 42.666667 0 0 1 3.541333-56.32zM512 85.333333c122.538667 0 227.84 73.813333 273.92 179.370667A213.333333 213.333333 0 0 1 725.333333 682.666667a42.666667 42.666667 0 0 1-4.992-85.034667L725.333333 597.333333a128 128 0 0 0 36.394667-250.794666l-38.144-11.264-15.914667-36.437334a213.376 213.376 0 0 0-407.296 58.026667l-7.381333 58.368-57.173333 13.824A85.418667 85.418667 0 0 0 256 597.333333h42.666667a42.666667 42.666667 0 0 1 0 85.333334H256a170.666667 170.666667 0 0 1-40.277333-336.554667A298.709333 298.709333 0 0 1 512 85.333333z' fill='white'/></svg>"];
    // upload.svg — newly replaced "send/cloud" icon supplied by the user.
    NSImage *uploadIcon = [self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M550.528 544.128L640 539.776V678.4l178.005333-178.005333L640 322.389333v129.024l-72.618667 10.922667c-155.52 23.424-251.733333 73.557333-312.874666 164.437333 84.906667-50.602667 178.261333-76.928 296.021333-82.645333zM554.666667 629.376c-27.392 1.322667-53.034667 3.968-77.141334 7.808l-8.192 1.365333c-136.704 23.637333-224.810667 87.168-310.954666 176.128-26.154667 27.008-51.882667 59.648-51.882667 59.648-14.848 18.176-24.021333 14.037333-20.352-9.173333 0 0 4.394667-31.402667 11.690667-65.792C144.938667 577.066667 250.581333 423.68 554.666667 377.941333V209.066667a38.4 38.4 0 0 1 65.536-27.136l291.328 291.285333a38.4 38.4 0 0 1 0 54.314667l-291.328 291.285333A38.4 38.4 0 0 1 554.666667 791.637333v-162.261333z' fill='white'/></svg>"];

    [self styleIconButton:_cancelButton image:cancelIcon];
    [self styleIconButton:_clipboardButton image:copyIcon];
    [self styleIconButton:_saveButton image:saveIcon];
    [self styleIconButton:_saveRemoteButton image:saveRemoteIcon];
    [self styleIconButton:_uploadButton image:uploadIcon];

    // Configure hover colors for the 5 action buttons. White is the shared
    // base; cancel turns red on hover and the rest turn blue, providing a
    // glance-able cue for destructive vs. constructive actions.
    NSColor *baseWhite = [NSColor whiteColor];
    NSColor *hoverRed  = [NSColor colorWithCalibratedRed:248.0/255.0 green:113.0/255.0 blue:113.0/255.0 alpha:1.0];
    NSColor *hoverBlue = [NSColor colorWithCalibratedRed:96.0/255.0  green:165.0/255.0 blue:250.0/255.0 alpha:1.0];
    SnipHoverButton *cancelHB     = (SnipHoverButton *)_cancelButton;
    SnipHoverButton *clipboardHB  = (SnipHoverButton *)_clipboardButton;
    SnipHoverButton *saveHB       = (SnipHoverButton *)_saveButton;
    SnipHoverButton *saveRemoteHB = (SnipHoverButton *)_saveRemoteButton;
    SnipHoverButton *uploadHB     = (SnipHoverButton *)_uploadButton;
    cancelHB.baseColor = baseWhite;     cancelHB.hoverColor = hoverRed;
    clipboardHB.baseColor = baseWhite;  clipboardHB.hoverColor = hoverBlue;
    saveHB.baseColor = baseWhite;       saveHB.hoverColor = hoverBlue;
    saveRemoteHB.baseColor = baseWhite; saveRemoteHB.hoverColor = hoverBlue;
    uploadHB.baseColor = baseWhite;     uploadHB.hoverColor = hoverBlue;

    [self styleIconButton:_penButton image:[self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M742.72 752.064c-29.696 28.576-42.976 48.96-42.976 77.12 0 98.4 143.776 142.464 229.728 65.536l-21.344-23.84c-66.976 59.936-176.384 26.4-176.384-41.696 0-16.832 9.28-31.104 33.184-54.08l13.44-12.736c61.024-58.016 75.328-102.72 29.312-179.84-43.84-73.44-96.8-88.288-162.784-50.56-47.232 27.04-68.64 47.456-208.32 190.4-70.624 72.288-153.92 77.088-217.344 26.784-59.712-47.328-79.872-127.36-42.176-188.64 24.512-39.84 62.656-72.896 136.64-124.48 5.952-4.192 27.04-18.816 31.104-21.664 12.16-8.448 21.536-15.104 30.4-21.536 107.552-78.272 140.8-139.136 86.048-219.904-76.992-113.472-202.304-90.016-367.744 61.472l21.6 23.616c153.088-140.224 256.896-159.616 319.68-67.104 41.632 61.408 16.896 106.688-78.4 176.032-8.64 6.304-17.92 12.832-29.888 21.184l-31.136 21.632c-77.504 54.08-117.984 89.184-145.536 133.984-46.784 76.032-22.176 173.664 49.536 230.496 76.224 60.416 177.952 54.56 260.096-29.504 136.16-139.36 158.08-160.224 201.312-184.96 50.656-28.96 84.416-19.52 119.424 39.168 36.896 61.824 27.52 91.392-23.808 140.16 0.096-0.064-10.976 10.368-13.632 12.96z' fill='white'/></svg>"]];
    [self styleIconButton:_rectButton image:[self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M96 96h832v832H96V96z m32 32v768h768V128H128z' fill='white'/></svg>"]];
    [self styleIconButton:_ellipseButton image:[self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><path d='M512 928c229.76 0 416-186.24 416-416S741.76 96 512 96 96 282.24 96 512s186.24 416 416 416z m0-32C299.936 896 128 724.064 128 512S299.936 128 512 128s384 171.936 384 384-171.936 384-384 384z' fill='white'/></svg>"]];
    [self styleIconButton:_colorButton image:nil];
    [self styleIconButton:_undoButton image:[self iconFromSVG:@"<svg viewBox='0 0 1024 1024' xmlns='http://www.w3.org/2000/svg'><g transform='translate(1024 0) scale(-1 1)'><path d='M838.976 288l-169.344-162.56a16 16 0 1 1 22.144-23.04l213.632 204.992-213.312 215.84a16 16 0 0 1-22.784-22.464L847.936 320H454.56c-164.192 0-297.28 128.96-297.28 288s133.088 288 297.28 288H704v32h-242.784C266.88 928 128 784.736 128 608S266.88 288 461.248 288h377.728z' fill='white'/></g></svg>"]];
    [_paletteView setWantsLayer:YES];
    [[_paletteView layer] setCornerRadius:8];
    [[_paletteView layer] setBackgroundColor:[[NSColor colorWithCalibratedWhite:0.11 alpha:0.96] CGColor]];
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

        [self drawAnnotations];
        [self drawResizeHandles];
    } else {
        NSRectFill(self.bounds);
    }
}

- (void)drawResizeHandles {
    NSArray *points = @[
        [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMinY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMidX(_selection), NSMinY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMinY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMidY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMaxY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMidX(_selection), NSMaxY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMaxY(_selection))],
        [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMidY(_selection))]
    ];
    [[NSColor whiteColor] setStroke];
    [[NSColor colorWithCalibratedRed:59.0/255.0 green:130.0/255.0 blue:246.0/255.0 alpha:1.0] setFill];
    for (NSValue *value in points) {
        NSPoint p = [value pointValue];
        NSRect handleRect = NSMakeRect(p.x - 5, p.y - 5, 10, 10);
        NSBezierPath *path = [NSBezierPath bezierPathWithRect:handleRect];
        [path fill];
        [path stroke];
    }
}

- (void)drawAnnotations {
    NSMutableArray *items = [NSMutableArray arrayWithArray:_annotations];
    if (_draftAnnotation != nil) {
        [items addObject:_draftAnnotation];
    }
    for (NSDictionary *item in items) {
        NSString *tool = item[@"tool"];
        NSColor *color = item[@"nsColor"];
        NSArray *points = item[@"points"];
        if (points.count == 0) {
            continue;
        }
        [color setStroke];
        NSBezierPath *path = [NSBezierPath bezierPath];
        [path setLineWidth:3];
        [path setLineCapStyle:NSLineCapStyleRound];
        [path setLineJoinStyle:NSLineJoinStyleRound];
        if ([tool isEqualToString:@"pen"]) {
            NSPoint first = [points[0] pointValue];
            [path moveToPoint:NSMakePoint(_selection.origin.x + first.x, _selection.origin.y + first.y)];
            for (NSUInteger i = 1; i < points.count; i++) {
                NSPoint point = [points[i] pointValue];
                [path lineToPoint:NSMakePoint(_selection.origin.x + point.x, _selection.origin.y + point.y)];
            }
            [path stroke];
        } else if (points.count >= 2) {
            NSPoint a = [points[0] pointValue];
            NSPoint b = [[points lastObject] pointValue];
            NSRect r = NSMakeRect(
                _selection.origin.x + MIN(a.x, b.x),
                _selection.origin.y + MIN(a.y, b.y),
                fabs(b.x - a.x),
                fabs(b.y - a.y));
            if ([tool isEqualToString:@"rect"]) {
                NSBezierPath *rectPath = [NSBezierPath bezierPathWithRect:r];
                [rectPath setLineWidth:3];
                [rectPath stroke];
            } else if ([tool isEqualToString:@"ellipse"]) {
                NSBezierPath *ellipsePath = [NSBezierPath bezierPathWithOvalInRect:r];
                [ellipsePath setLineWidth:3];
                [ellipsePath stroke];
            }
        }
    }
}

- (NSString *)resizeHandleAtPoint:(NSPoint)p {
    if (!_hasSelection) {
        return nil;
    }
    CGFloat t = 9;
    NSDictionary *handles = @{
        @"nw": [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMinY(_selection))],
        @"n": [NSValue valueWithPoint:NSMakePoint(NSMidX(_selection), NSMinY(_selection))],
        @"ne": [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMinY(_selection))],
        @"e": [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMidY(_selection))],
        @"se": [NSValue valueWithPoint:NSMakePoint(NSMaxX(_selection), NSMaxY(_selection))],
        @"s": [NSValue valueWithPoint:NSMakePoint(NSMidX(_selection), NSMaxY(_selection))],
        @"sw": [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMaxY(_selection))],
        @"w": [NSValue valueWithPoint:NSMakePoint(NSMinX(_selection), NSMidY(_selection))]
    };
    for (NSString *key in handles) {
        NSPoint hp = [handles[key] pointValue];
        if (fabs(p.x - hp.x) <= t && fabs(p.y - hp.y) <= t) {
            return key;
        }
    }
    BOOL nearX = p.x >= NSMinX(_selection) - t && p.x <= NSMaxX(_selection) + t;
    BOOL nearY = p.y >= NSMinY(_selection) - t && p.y <= NSMaxY(_selection) + t;
    if (nearX && fabs(p.y - NSMinY(_selection)) <= t) return @"n";
    if (nearX && fabs(p.y - NSMaxY(_selection)) <= t) return @"s";
    if (nearY && fabs(p.x - NSMinX(_selection)) <= t) return @"w";
    if (nearY && fabs(p.x - NSMaxX(_selection)) <= t) return @"e";
    return nil;
}

- (NSPoint)localPoint:(NSPoint)p {
    return NSMakePoint(
        MAX(0, MIN(p.x - _selection.origin.x, _selection.size.width)),
        MAX(0, MIN(p.y - _selection.origin.y, _selection.size.height)));
}

// Translate a selection rectangle expressed in view-local coordinates into
// the global top-left screen coordinate system used by
// `/usr/sbin/screencapture -R`.
//
// Why this is needed:
//   - The overlay window is created with the host screen's AppKit frame and
//     its flipped content view yields top-left local coordinates relative
//     to that screen only.
//   - macOS combines all attached displays into a single virtual coordinate
//     space whose origin is the top-left corner of the primary display
//     (Y grows downward). `screencapture -R` interprets its arguments in
//     that space.
//   - Without this translation, a selection on a secondary display is sent
//     as if it were on the primary, so screencapture crops the wrong area.
//
// Translation steps:
//   1. globalX = screenFrame.origin.x + sel.origin.x
//      (NSScreen frame origin.x is already in primary-anchored global X.)
//   2. globalY = (primaryHeight - (screenFrame.origin.y + screenFrame.size.height)) + sel.origin.y
//      Converts the screen's AppKit bottom-left origin to a top-left global
//      origin, then adds the already-top-left local Y inside the view.
- (NSRect)globalRectForSelection:(NSRect)sel {
    NSScreen *screen = [nativeOverlayWindow screen];
    if (screen == nil) {
        screen = [NSScreen mainScreen];
    }
    if (screen == nil) {
        return sel;
    }
    NSRect screenFrame = [screen frame];
    NSScreen *primary = [[NSScreen screens] firstObject];
    CGFloat primaryHeight = primary != nil ? [primary frame].size.height : screenFrame.size.height;
    CGFloat globalX = screenFrame.origin.x + sel.origin.x;
    CGFloat globalY = (primaryHeight - (screenFrame.origin.y + screenFrame.size.height)) + sel.origin.y;
    return NSMakeRect(globalX, globalY, sel.size.width, sel.size.height);
}

- (void)mouseDown:(NSEvent *)event {
    NSPoint p = [self convertPoint:[event locationInWindow] fromView:nil];
    if ([event clickCount] == 2 && _hasSelection && NSPointInRect(p, _selection)) {
        [self copySelection];
        return;
    }
    NSString *handle = [self resizeHandleAtPoint:p];
    if (handle != nil) {
        _resizing = YES;
        _creating = NO;
        _moving = NO;
        _annotating = NO;
        _resizeHandle = handle;
        _resizeStart = _selection;
    } else if (_hasSelection && NSPointInRect(p, _selection) && _activeTool != nil) {
        _annotating = YES;
        _creating = NO;
        _moving = NO;
        _resizing = NO;
        NSPoint local = [self localPoint:p];
        _draftAnnotation = [@{
            @"tool": _activeTool,
            @"color": [self hexForColor:_activeColor],
            @"nsColor": _activeColor,
            @"points": [NSMutableArray arrayWithObject:[NSValue valueWithPoint:local]]
        } mutableCopy];
    } else if (_hasSelection && NSPointInRect(p, _selection)) {
        _moving = YES;
        _creating = NO;
        _resizing = NO;
        _annotating = NO;
        _moveOffset = NSMakePoint(p.x - _selection.origin.x, p.y - _selection.origin.y);
    } else {
        _creating = YES;
        _moving = NO;
        _resizing = NO;
        _annotating = NO;
        _hasSelection = YES;
        _anchor = p;
        _selection = NSMakeRect(p.x, p.y, 0, 0);
        [_annotations removeAllObjects];
        _draftAnnotation = nil;
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
    } else if (_resizing) {
        CGFloat left = NSMinX(_resizeStart);
        CGFloat top = NSMinY(_resizeStart);
        CGFloat right = NSMaxX(_resizeStart);
        CGFloat bottom = NSMaxY(_resizeStart);
        if ([_resizeHandle containsString:@"w"]) left = p.x;
        if ([_resizeHandle containsString:@"e"]) right = p.x;
        if ([_resizeHandle containsString:@"n"]) top = p.y;
        if ([_resizeHandle containsString:@"s"]) bottom = p.y;
        left = MAX(0, MIN(left, self.bounds.size.width));
        right = MAX(0, MIN(right, self.bounds.size.width));
        top = MAX(0, MIN(top, self.bounds.size.height));
        bottom = MAX(0, MIN(bottom, self.bounds.size.height));
        _selection = NSMakeRect(MIN(left, right), MIN(top, bottom), fabs(right - left), fabs(bottom - top));
    } else if (_annotating && _draftAnnotation != nil) {
        NSMutableArray *points = _draftAnnotation[@"points"];
        NSPoint local = [self localPoint:p];
        if ([_activeTool isEqualToString:@"pen"]) {
            [points addObject:[NSValue valueWithPoint:local]];
        } else {
            if (points.count == 1) {
                [points addObject:[NSValue valueWithPoint:local]];
            } else {
                points[1] = [NSValue valueWithPoint:local];
            }
        }
    }
    [self syncControls];
    [self setNeedsDisplay:YES];
}

- (void)mouseUp:(NSEvent *)event {
    _creating = NO;
    _moving = NO;
    _resizing = NO;
    if (_annotating && _draftAnnotation != nil) {
        [_annotations addObject:_draftAnnotation];
        _draftAnnotation = nil;
    }
    _annotating = NO;
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
    [_clipboardButton setHidden:!visible];
    [_saveButton setHidden:!visible];
    [_saveRemoteButton setHidden:!visible];
    [_uploadButton setHidden:!visible];
    [_actionToolbarBg setHidden:!visible];
    [_penButton setHidden:!visible];
    [_rectButton setHidden:!visible];
    [_ellipseButton setHidden:!visible];
    [_colorButton setHidden:!visible];
    [_undoButton setHidden:!visible];
    [_sizeLabel setHidden:!visible];
    [_hintLabel setHidden:_hasSelection];

    if (!visible) {
        return;
    }

    [_sizeLabel setStringValue:[NSString stringWithFormat:@"%.0f × %.0f", _selection.size.width, _selection.size.height]];
    [_sizeLabel setFrame:NSMakeRect(_selection.origin.x, MAX(0, _selection.origin.y - 26), 110, 22)];

    // Action toolbar layout — 5 icon buttons, each 28x28, separated by 8px,
    // with 4px outer padding. Total = 5*28 + 4*8 + 2*4 = 180px wide.
    CGFloat actionBtnSize = 28;
    CGFloat actionGap = 8;
    CGFloat actionPad = 4;
    NSInteger actionCount = 5;
    CGFloat toolbarW = actionPad * 2 + actionBtnSize * actionCount + actionGap * (actionCount - 1);
    CGFloat toolbarH = 40;
    CGFloat x = _selection.origin.x + _selection.size.width - toolbarW;
    CGFloat y = _selection.origin.y + _selection.size.height + 8;
    if (y + toolbarH > self.bounds.size.height) {
        y = _selection.origin.y + _selection.size.height - toolbarH - 8;
    }
    x = MAX(0, MIN(x, self.bounds.size.width - toolbarW));

    [_actionToolbarBg setFrame:NSMakeRect(x, y, toolbarW, toolbarH)];

    CGFloat btnY = y + (toolbarH - actionBtnSize) / 2;
    CGFloat baseX = x + actionPad;
    [_cancelButton     setFrame:NSMakeRect(baseX + (actionBtnSize + actionGap) * 0, btnY, actionBtnSize, actionBtnSize)];
    [_clipboardButton  setFrame:NSMakeRect(baseX + (actionBtnSize + actionGap) * 1, btnY, actionBtnSize, actionBtnSize)];
    [_saveButton       setFrame:NSMakeRect(baseX + (actionBtnSize + actionGap) * 2, btnY, actionBtnSize, actionBtnSize)];
    [_saveRemoteButton setFrame:NSMakeRect(baseX + (actionBtnSize + actionGap) * 3, btnY, actionBtnSize, actionBtnSize)];
    [_uploadButton     setFrame:NSMakeRect(baseX + (actionBtnSize + actionGap) * 4, btnY, actionBtnSize, actionBtnSize)];

    CGFloat markW = 170;
    CGFloat markX = MAX(0, MIN(_selection.origin.x, self.bounds.size.width - markW));
    [_penButton setFrame:NSMakeRect(markX + 4, y + 4, 28, 28)];
    [_rectButton setFrame:NSMakeRect(markX + 36, y + 4, 28, 28)];
    [_ellipseButton setFrame:NSMakeRect(markX + 68, y + 4, 28, 28)];
    [_colorButton setFrame:NSMakeRect(markX + 104, y + 7, 22, 22)];
    [_undoButton setFrame:NSMakeRect(markX + 134, y + 4, 28, 28)];
    [[_colorButton layer] setBackgroundColor:[_activeColor CGColor]];
    [_undoButton setEnabled:_annotations.count > 0];
    [_paletteView setFrame:NSMakeRect(markX, MAX(0, y - 78), 150, 70)];
    [self updateToolButtonStates];
}

- (void)updateToolButtonStates {
    NSDictionary *buttons = @{@"pen": _penButton, @"rect": _rectButton, @"ellipse": _ellipseButton};
    for (NSString *tool in buttons) {
        NSButton *button = buttons[tool];
        NSColor *bg = [tool isEqualToString:_activeTool]
            ? [NSColor colorWithCalibratedWhite:1.0 alpha:0.14]
            : [NSColor clearColor];
        [[button layer] setBackgroundColor:[bg CGColor]];
    }
}

- (void)toggleTool:(NSString *)tool {
    _activeTool = [_activeTool isEqualToString:tool] ? nil : tool;
    [self syncControls];
}
- (void)selectPen { [self toggleTool:@"pen"]; }
- (void)selectRect { [self toggleTool:@"rect"]; }
- (void)selectEllipse { [self toggleTool:@"ellipse"]; }
- (void)undoAnnotation {
    if (_annotations.count > 0) {
        [_annotations removeLastObject];
        [self syncControls];
        [self setNeedsDisplay:YES];
    }
}

- (void)togglePalette {
    [_paletteView setHidden:![_paletteView isHidden]];
    if (![_paletteView isHidden] && _paletteView.subviews.count == 0) {
        NSArray *colors = @[
            [NSColor colorWithCalibratedRed:239.0/255.0 green:68.0/255.0 blue:68.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:249.0/255.0 green:115.0/255.0 blue:22.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:250.0/255.0 green:204.0/255.0 blue:21.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:34.0/255.0 green:197.0/255.0 blue:94.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:59.0/255.0 green:130.0/255.0 blue:246.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:139.0/255.0 green:92.0/255.0 blue:246.0/255.0 alpha:1.0],
            [NSColor colorWithCalibratedRed:236.0/255.0 green:72.0/255.0 blue:153.0/255.0 alpha:1.0],
            [NSColor whiteColor],
            [NSColor colorWithCalibratedWhite:0.07 alpha:1.0]
        ];
        for (NSUInteger i = 0; i < colors.count; i++) {
            NSButton *button = [NSButton buttonWithTitle:@"" target:self action:@selector(selectPaletteColor:)];
            [button setBordered:NO];
            [button setWantsLayer:YES];
            [[button layer] setCornerRadius:4];
            [[button layer] setBackgroundColor:[colors[i] CGColor]];
            [button setTag:(NSInteger)i];
            [button setFrame:NSMakeRect(8 + (i % 5) * 28, 38 - (i / 5) * 28, 22, 22)];
            [_paletteView addSubview:button];
        }
        objc_setAssociatedObject(_paletteView, "snapgoColors", colors, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
    }
}

- (void)selectPaletteColor:(NSButton *)sender {
    NSArray *colors = objc_getAssociatedObject(_paletteView, "snapgoColors");
    if (sender.tag >= 0 && sender.tag < (NSInteger)colors.count) {
        _activeColor = colors[(NSUInteger)sender.tag];
        [_paletteView setHidden:YES];
        [self syncControls];
    }
}

- (NSString *)annotationsJSON {
    NSMutableArray *payload = [NSMutableArray array];
    for (NSDictionary *item in _annotations) {
        NSMutableArray *points = [NSMutableArray array];
        for (NSValue *value in item[@"points"]) {
            NSPoint p = [value pointValue];
            [points addObject:@{@"x": @(p.x), @"y": @(p.y)}];
        }
        [payload addObject:@{@"tool": item[@"tool"], @"color": item[@"color"], @"points": points}];
    }
    NSData *data = [NSJSONSerialization dataWithJSONObject:payload options:0 error:nil];
    if (data == nil) {
        return @"[]";
    }
    return [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
}

- (void)closeOverlayWindow {
    [nativeOverlayWindow orderOut:nil];
    if (nativeOverlayKeyMonitor != nil) {
        [NSEvent removeMonitor:nativeOverlayKeyMonitor];
        nativeOverlayKeyMonitor = nil;
    }
    nativeOverlayWindow = nil;
}

- (void)confirmSelection {
    if (!_hasSelection) {
        return;
    }
    NSRect r = [self globalRectForSelection:_selection];
    [self closeOverlayWindow];
    NSString *json = [self annotationsJSON];
    nativeOverlayConfirm((int)llround(r.origin.x), (int)llround(r.origin.y), (int)llround(r.size.width), (int)llround(r.size.height), [json UTF8String]);
}

- (void)copySelection {
    if (!_hasSelection) {
        return;
    }
    NSRect r = [self globalRectForSelection:_selection];
    [self closeOverlayWindow];
    NSString *json = [self annotationsJSON];
    nativeOverlayCopy((int)llround(r.origin.x), (int)llround(r.origin.y), (int)llround(r.size.width), (int)llround(r.size.height), [json UTF8String]);
}

- (void)saveSelection {
    if (!_hasSelection) {
        return;
    }
    NSRect r = [self globalRectForSelection:_selection];
    NSString *json = [self annotationsJSON];
    [self closeOverlayWindow];

    NSOpenPanel *panel = [NSOpenPanel openPanel];
    [panel setTitle:@"Save screenshot to folder"];
    [panel setCanChooseFiles:NO];
    [panel setCanChooseDirectories:YES];
    [panel setAllowsMultipleSelection:NO];
    [panel setCanCreateDirectories:YES];
    [panel beginWithCompletionHandler:^(NSModalResponse result) {
        if (result != NSModalResponseOK || [panel URL] == nil) {
            nativeOverlayCancel();
            return;
        }
        NSString *dir = [[panel URL] path];
        nativeOverlaySave((int)llround(r.origin.x), (int)llround(r.origin.y), (int)llround(r.size.width), (int)llround(r.size.height), [json UTF8String], [dir UTF8String]);
    }];
}

- (void)cancelSelection {
    [self closeOverlayWindow];
    nativeOverlayCancel();
}

// `saveRemoteSelection` triggers the SCP upload flow. The overlay closes
// immediately so the user can keep working while the transfer runs in the
// background; success / failure surfaces through the regular Toast events.
- (void)saveRemoteSelection {
    if (!_hasSelection) {
        return;
    }
    NSRect r = [self globalRectForSelection:_selection];
    [self closeOverlayWindow];
    NSString *json = [self annotationsJSON];
    nativeOverlaySaveRemote((int)llround(r.origin.x), (int)llround(r.origin.y), (int)llround(r.size.width), (int)llround(r.size.height), [json UTF8String]);
}
@end

// screenContainingCursor returns the NSScreen the cursor is currently on,
// falling back to the main screen and then the first attached screen.
//
// Rationale: when an external display is attached, users expect the capture
// overlay to appear on whichever display they were just working on.
// `[NSEvent mouseLocation]` reports the cursor in the same global Cocoa
// (bottom-left-origin) screen space used by `NSScreen.frame`, so a simple
// `NSPointInRect` test against each screen's frame yields the right one.
// Iteration over `[NSScreen screens]` is cheap (typically <=4 displays).
static NSScreen *snipScreenContainingCursor(void) {
    NSPoint cursor = [NSEvent mouseLocation];
    for (NSScreen *candidate in [NSScreen screens]) {
        if (NSPointInRect(cursor, [candidate frame])) {
            return candidate;
        }
    }
    NSScreen *fallback = [NSScreen mainScreen];
    if (fallback != nil) {
        return fallback;
    }
    return [[NSScreen screens] firstObject];
}

static void snipShowNativeOverlay(int width, int height) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSScreen *screen = snipScreenContainingCursor();
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
