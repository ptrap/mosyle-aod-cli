enum {
    AOD_AUTH_PENDING = 0,
    AOD_AUTH_SUCCESS,
    AOD_AUTH_CANCELED,
    AOD_AUTH_UNAVAILABLE,
    AOD_AUTH_FAILED
};

void *aod_auth_begin(const char *reason);
int aod_auth_poll(void *handle);
void aod_auth_release(void *handle);
