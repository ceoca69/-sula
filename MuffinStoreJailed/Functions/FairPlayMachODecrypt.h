#ifndef FairPlayMachODecrypt_h
#define FairPlayMachODecrypt_h

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Returns 0 when encrypted Mach-O slices were decrypted, 1 when the file
// contains no encrypted slices, and -1 when decryption failed.
int32_t PSFairPlayDecryptMachO(const char *sourcePath, const char *outputPath, char *error, size_t errorLength);
bool PSIsMachOFile(const char *path);

#ifdef __cplusplus
}
#endif

#endif
