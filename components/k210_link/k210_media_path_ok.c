#include <string.h>
#include "k210_media.h"

#define K210_PATH_MAX 127u
#define K210_NAME_MAX 64u

static int printable_safe(unsigned char c)
{
    return c >= 0x21 && c <= 0x7E && c != '"' && c != '\\';
}

int k210_media_name_ok(const char *name)
{
    if (!name || !*name) {
        return 0;
    }
    size_t n = strlen(name);
    if (n > K210_NAME_MAX || strstr(name, "..")) {
        return 0;
    }
    for (size_t i = 0; i < n; i++) {
        if (!printable_safe((unsigned char)name[i]) || name[i] == '/') {
            return 0;
        }
    }
    return 1;
}

int k210_media_path_ok(const char *path)
{
    if (!path || path[0] != '/') {
        return 0;
    }
    size_t n = strlen(path);
    if (n < 2 || n > K210_PATH_MAX || strstr(path, "..")) {
        return 0;
    }
    for (size_t i = 0; i < n; i++) {
        if (!printable_safe((unsigned char)path[i])) {
            return 0;
        }
    }
    return 1;
}
