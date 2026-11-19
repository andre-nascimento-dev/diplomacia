#import <Cocoa/Cocoa.h>

void trayOpen(void);
void trayQuit(void);

@interface Tray : NSObject
@end

@implementation Tray
- (void)open:(id)sender {
    [NSApp activateIgnoringOtherApps:YES];
    trayOpen();
}
- (void)quit:(id)sender { trayQuit(); }
@end

void startTray(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        // Accessory: sem ícone no Dock, só na barra de menus.
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        Tray *tray = [Tray new];
        NSStatusItem *item = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength] retain];
        NSImage *icon = [NSImage imageWithSystemSymbolName:@"bubble.left.and.bubble.right.fill" accessibilityDescription:@"DiplomacIA"];
        icon.template = YES;
        item.button.image = icon;
        NSMenu *menu = [NSMenu new];
        [menu addItemWithTitle:@"Abrir DiplomacIA" action:@selector(open:) keyEquivalent:@""].target = tray;
        [menu addItem:[NSMenuItem separatorItem]];
        [menu addItemWithTitle:@"Sair" action:@selector(quit:) keyEquivalent:@"q"].target = tray;
        item.menu = menu;
    });
}
