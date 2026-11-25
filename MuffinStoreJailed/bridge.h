//
//  bridge.h
//  dirtyZero
//
//  Created by Skadz on 5/20/25.
//

#ifndef bridge_h
#define bridge_h

#import <Foundation/Foundation.h>
#import "../GoBridge/libPancakeIPATool.h"
#import "Functions/FairPlayMachODecrypt.h"

char *PancakeIPAToolLogin(char *raw);
char *PancakeIPAToolListVersions(char *raw);
char *PancakeIPAToolDownload(char *raw);
void PancakeIPAToolFree(char *value);

NSString * _Nullable PSResolveInstalledAppBundlePath(NSString *bundleIdentifier);

@interface LSApplicationWorkspace : NSObject
+ (instancetype)defaultWorkspace;
- (bool)openApplicationWithBundleID:(NSString*)bundleID;
@end

#endif /* bridge_h */
