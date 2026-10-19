#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include "login_darwin.h"
#include "_cgo_export.h"

static NSWindow *gWindow;
static WKWebView *gWeb;
static NSString *gHost;
static NSTimer *gTimer;

static void wakeRunLoop(void) {
	NSEvent *e = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
	                                location:NSZeroPoint
	                           modifierFlags:0
	                               timestamp:0
	                            windowNumber:0
	                                 context:nil
	                                 subtype:0
	                                   data1:0
	                                   data2:0];
	[NSApp postEvent:e atStart:YES];
}

static BOOL hostMatches(NSString *domain) {
	NSString *d = [domain hasPrefix:@"."] ? [domain substringFromIndex:1] : domain;
	return [gHost isEqualToString:d] || [gHost hasSuffix:[@"." stringByAppendingString:d]];
}

static void pollCookies(void) {
	if (gWeb == nil) {
		return;
	}
	[gWeb.configuration.websiteDataStore.httpCookieStore getAllCookies:^(NSArray<NSHTTPCookie *> *cookies) {
		NSMutableString *s = [NSMutableString string];
		for (NSHTTPCookie *c in cookies) {
			if (hostMatches(c.domain)) {
				[s appendFormat:@"%@\t%@\n", c.name, c.value];
			}
		}
		canvasCookies((char *)[s UTF8String]);
	}];
}

@interface CLDelegate : NSObject <NSWindowDelegate, WKNavigationDelegate, WKUIDelegate>
@end

@implementation CLDelegate
- (void)windowWillClose:(NSNotification *)n {
	[NSApp stop:nil];
	wakeRunLoop();
}

- (void)webView:(WKWebView *)wv didFinishNavigation:(WKNavigation *)nav {
	pollCookies();
}

// SSO pages sometimes open links with target=_blank; keep them in this window.
- (WKWebView *)webView:(WKWebView *)wv
    createWebViewWithConfiguration:(WKWebViewConfiguration *)cfg
               forNavigationAction:(WKNavigationAction *)action
                    windowFeatures:(WKWindowFeatures *)features {
	if (!action.targetFrame.isMainFrame) {
		[wv loadRequest:action.request];
	}
	return nil;
}
@end

// Without a main menu, Cmd-C/Cmd-V/Cmd-A don't reach the web view, which
// makes pasting a password impossible.
static void installMenu(NSString *title) {
	NSMenu *bar = [NSMenu new];

	NSMenuItem *appItem = [NSMenuItem new];
	NSMenu *appMenu = [NSMenu new];
	[appMenu addItemWithTitle:[@"Close " stringByAppendingString:title] action:@selector(performClose:) keyEquivalent:@"q"];
	[appMenu addItemWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"];
	appItem.submenu = appMenu;
	[bar addItem:appItem];

	NSMenuItem *editItem = [NSMenuItem new];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
	[edit addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
	[edit addItem:[NSMenuItem separatorItem]];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	editItem.submenu = edit;
	[bar addItem:editItem];

	NSApp.mainMenu = bar;
}

void canvas_login_run(const char *url, const char *host, const char *title) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
		NSString *t = @(title);
		installMenu(t);
		gHost = @(host);

		CLDelegate *del = [CLDelegate new];
		WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
		cfg.websiteDataStore = [WKWebsiteDataStore nonPersistentDataStore];

		NSRect frame = NSMakeRect(0, 0, 1000, 800);
		gWindow = [[NSWindow alloc] initWithContentRect:frame
		                                      styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
		                                                NSWindowStyleMaskResizable | NSWindowStyleMaskMiniaturizable
		                                        backing:NSBackingStoreBuffered
		                                          defer:NO];
		gWindow.releasedWhenClosed = NO;
		gWindow.title = t;
		gWindow.delegate = del;
		[gWindow center];

		gWeb = [[WKWebView alloc] initWithFrame:frame configuration:cfg];
		gWeb.navigationDelegate = del;
		gWeb.UIDelegate = del;
		gWindow.contentView = gWeb;
		[gWeb loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:@(url)]]];

		gTimer = [NSTimer scheduledTimerWithTimeInterval:1.0
		                                         repeats:YES
		                                           block:^(NSTimer *timer) {
			                                           pollCookies();
		                                           }];

		[gWindow makeKeyAndOrderFront:nil];
		[NSApp activateIgnoringOtherApps:YES];
		[NSApp run];

		[gTimer invalidate];
		gTimer = nil;
		gWindow.delegate = nil;
		gWeb.navigationDelegate = nil;
		gWeb.UIDelegate = nil;
		[gWindow orderOut:nil];
		[gWindow close];
		gWeb = nil;
		gWindow = nil;
		[NSApp setActivationPolicy:NSApplicationActivationPolicyProhibited];
	}
}

void canvas_login_stop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp stop:nil];
		wakeRunLoop();
	});
}
