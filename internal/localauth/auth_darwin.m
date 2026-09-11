//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#include "auth.h"

@interface AODAuthentication : NSObject
@property(nonatomic, strong) LAContext *context;
@property(nonatomic, strong) dispatch_semaphore_t completed;
@property(nonatomic) int result;
@end

@implementation AODAuthentication
@end

void *aod_auth_begin(const char *reason) {
    @autoreleasepool {
        AODAuthentication *auth = [AODAuthentication new];
        auth.context = [LAContext new];
        auth.completed = dispatch_semaphore_create(0);
        auth.result = AOD_AUTH_PENDING;
        // Never intentionally reuse a previous Touch ID authentication.
        auth.context.touchIDAuthenticationAllowableReuseDuration = 0;
        NSError *error = nil;
        if (![auth.context canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&error]) {
            auth.result = AOD_AUTH_UNAVAILABLE;
            dispatch_semaphore_signal(auth.completed);
        } else {
            [auth.context evaluatePolicy:LAPolicyDeviceOwnerAuthentication
                         localizedReason:[NSString stringWithUTF8String:reason]
                                   reply:^(BOOL success, NSError *error) {
                if (success) {
                    auth.result = AOD_AUTH_SUCCESS;
                } else if ([error.domain isEqualToString:LAErrorDomain] &&
                           (error.code == LAErrorUserCancel || error.code == LAErrorSystemCancel ||
                            error.code == LAErrorAppCancel)) {
                    auth.result = AOD_AUTH_CANCELED;
                } else {
                    auth.result = AOD_AUTH_FAILED;
                }
                dispatch_semaphore_signal(auth.completed);
            }];
        }
        return (__bridge_retained void *)auth;
    }
}

int aod_auth_poll(void *handle) {
    @autoreleasepool {
        AODAuthentication *auth = (__bridge AODAuthentication *)handle;
        // The semaphore synchronizes the callback's result with the Go caller.
        if (dispatch_semaphore_wait(auth.completed, DISPATCH_TIME_NOW) != 0) {
            return AOD_AUTH_PENDING;
        }
        return auth.result;
    }
}

void aod_auth_release(void *handle) {
    @autoreleasepool {
        AODAuthentication *auth = (__bridge_transfer AODAuthentication *)handle;
        [auth.context invalidate];
        auth.context = nil;
        // A pending reply retains auth until it finishes after invalidation.
    }
}
