#import <Foundation/Foundation.h>
#import <objc/message.h>

// Resolve one explicitly named installed application. We deliberately do not
// enumerate installed apps; iOS restricts broad LaunchServices enumeration.
NSString * _Nullable PSResolveInstalledAppBundlePath(NSString *bundleIdentifier) {
    if (bundleIdentifier.length == 0) return nil;

    Class proxyClass = NSClassFromString(@"LSApplicationProxy");
    SEL proxySelector = NSSelectorFromString(@"applicationProxyForIdentifier:");
    if (proxyClass && [proxyClass respondsToSelector:proxySelector]) {
        @try {
            typedef id (*ProxyFunction)(id, SEL, id);
            id proxy = ((ProxyFunction)objc_msgSend)(proxyClass, proxySelector, bundleIdentifier);
            NSURL *bundleURL = [proxy valueForKey:@"bundleURL"];
            NSString *path = bundleURL.path;
            if (path.length && [[NSFileManager defaultManager] fileExistsAtPath:path]) {
                NSDictionary *info = [NSDictionary dictionaryWithContentsOfFile:[path stringByAppendingPathComponent:@"Info.plist"]];
                if ([info[@"CFBundleIdentifier"] isEqualToString:bundleIdentifier]) {
                    NSLog(@"[Decrypt] Resolved %@ through LSApplicationProxy: %@", bundleIdentifier, path);
                    return path;
                }
            }
        } @catch (NSException *exception) {
            NSLog(@"[Decrypt] LSApplicationProxy exception for %@: %@", bundleIdentifier, exception.reason ?: @"unknown");
        }
    }

    // Some iOS builds expose the application record resolver even when the
    // LaunchServices proxy selector is unavailable.
    Class recordClass = NSClassFromString(@"LSApplicationRecord");
    SEL initSelector = NSSelectorFromString(@"initWithBundleIdentifier:allowPlaceholder:error:");
    if (recordClass && [recordClass instancesRespondToSelector:initSelector]) {
        @try {
            NSError *recordError = nil;
            // This is an init-family method called through a function pointer.
            // Tell ARC that init consumes the +1 receiver and returns a +1 object;
            // otherwise the dynamic call can over-release the LSApplicationRecord.
            typedef id (*RecordInitFunction)(__attribute__((ns_consumed)) id, SEL,
                                             NSString *, BOOL,
                                             NSError * __autoreleasing *)
                __attribute__((ns_returns_retained));
            RecordInitFunction initFunction =
                (RecordInitFunction)[recordClass instanceMethodForSelector:initSelector];
            id record = initFunction([recordClass alloc], initSelector,
                                     bundleIdentifier, NO, &recordError);
            NSURL *bundleURL = record ? [record valueForKey:@"URL"] : nil;
            NSString *path = bundleURL.path;
            if (path.length && [[NSFileManager defaultManager] fileExistsAtPath:path]) {
                NSDictionary *info = [NSDictionary dictionaryWithContentsOfFile:[path stringByAppendingPathComponent:@"Info.plist"]];
                if ([info[@"CFBundleIdentifier"] isEqualToString:bundleIdentifier]) {
                    NSLog(@"[Decrypt] Resolved %@ through LSApplicationRecord: %@", bundleIdentifier, path);
                    return path;
                }
            }
            NSLog(@"[Decrypt] LSApplicationRecord could not resolve %@: %@", bundleIdentifier, recordError.localizedDescription ?: @"no URL");
        } @catch (NSException *exception) {
            NSLog(@"[Decrypt] LSApplicationRecord exception for %@: %@", bundleIdentifier, exception.reason ?: @"unknown");
        }
    }
    return nil;
}
