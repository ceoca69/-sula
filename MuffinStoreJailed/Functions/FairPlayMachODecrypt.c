#include "FairPlayMachODecrypt.h"

// Independent implementation of the public mremap_encrypted approach
// described in FairPlayed research. No FairPlayed source files are vendored.

#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <libkern/OSByteOrder.h>
#include <math.h>
#include <mach/mach.h>
#include <mach-o/fat.h>
#include <mach-o/loader.h>
#include <mach/machine.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#define PS_MAX_SLICES 16
#define PS_MAX_LOAD_COMMANDS_SIZE (16U * 1024U * 1024U)

typedef struct {
    uint64_t sliceOffset;
    uint64_t sliceSize;
    uint64_t encryptionCommandOffset;
    uint64_t codeSignatureOffset;
    uint64_t codeSignatureSize;
    uint32_t cryptOffset;
    uint32_t cryptSize;
    uint32_t cryptID;
    uint32_t cpuType;
    uint32_t cpuSubtype;
    bool hasEncryptionCommand;
} PSSlice;

static void ps_set_error(char *error, size_t capacity, const char *message) {
    if (error && capacity) snprintf(error, capacity, "%s", message ? message : "Unknown error");
}

static bool ps_pread_all(int fd, void *buffer, size_t length, off_t offset) {
    uint8_t *cursor = (uint8_t *)buffer;
    while (length) {
        ssize_t count = pread(fd, cursor, length, offset);
        if (count < 0 && errno == EINTR) continue;
        if (count <= 0) return false;
        cursor += count;
        length -= (size_t)count;
        offset += count;
    }
    return true;
}

static bool ps_pwrite_all(int fd, const void *buffer, size_t length, off_t offset) {
    const uint8_t *cursor = (const uint8_t *)buffer;
    while (length) {
        ssize_t count = pwrite(fd, cursor, length, offset);
        if (count < 0 && errno == EINTR) continue;
        if (count <= 0) return false;
        cursor += count;
        length -= (size_t)count;
        offset += count;
    }
    return true;
}

bool PSIsMachOFile(const char *path) {
    if (!path) return false;
    int fd = open(path, O_RDONLY);
    if (fd < 0) return false;
    uint32_t magic = 0;
    bool readOK = ps_pread_all(fd, &magic, sizeof(magic), 0);
    close(fd);
    if (!readOK) return false;
    return magic == MH_MAGIC_64 || magic == FAT_CIGAM || magic == FAT_CIGAM_64;
}

static int ps_parse_thin(int fd, uint64_t offset, uint64_t size, PSSlice *slice,
                         char *error, size_t errorLength) {
    struct mach_header_64 header;
    if (size < sizeof(header) || !ps_pread_all(fd, &header, sizeof(header), (off_t)offset)) {
        ps_set_error(error, errorLength, "Could not read Mach-O header");
        return -1;
    }
    if (header.magic != MH_MAGIC_64) {
        ps_set_error(error, errorLength, "Only 64-bit little-endian Mach-O slices are supported");
        return -1;
    }
    if (header.sizeofcmds > PS_MAX_LOAD_COMMANDS_SIZE ||
        sizeof(header) + (uint64_t)header.sizeofcmds > size) {
        ps_set_error(error, errorLength, "Mach-O load-command area is invalid");
        return -1;
    }

    memset(slice, 0, sizeof(*slice));
    slice->sliceOffset = offset;
    slice->sliceSize = size;
    slice->cpuType = (uint32_t)header.cputype;
    slice->cpuSubtype = (uint32_t)header.cpusubtype;

    uint8_t *commands = (uint8_t *)malloc(header.sizeofcmds ? header.sizeofcmds : 1);
    if (!commands) {
        ps_set_error(error, errorLength, "Not enough memory to parse Mach-O commands");
        return -1;
    }
    if (header.sizeofcmds && !ps_pread_all(fd, commands, header.sizeofcmds,
                                            (off_t)(offset + sizeof(header)))) {
        free(commands);
        ps_set_error(error, errorLength, "Could not read Mach-O load commands");
        return -1;
    }

    uint32_t position = 0;
    for (uint32_t index = 0; index < header.ncmds; index++) {
        if (position > header.sizeofcmds || header.sizeofcmds - position < sizeof(struct load_command)) break;
        struct load_command *command = (struct load_command *)(commands + position);
        if (command->cmdsize < sizeof(struct load_command) ||
            command->cmdsize > header.sizeofcmds - position) break;

        if (command->cmd == LC_ENCRYPTION_INFO_64 || command->cmd == LC_ENCRYPTION_INFO) {
            if (command->cmdsize >= sizeof(struct encryption_info_command)) {
                struct encryption_info_command *encryption = (struct encryption_info_command *)command;
                slice->hasEncryptionCommand = true;
                slice->encryptionCommandOffset = offset + sizeof(header) + position;
                slice->cryptOffset = encryption->cryptoff;
                slice->cryptSize = encryption->cryptsize;
                slice->cryptID = encryption->cryptid;
            }
        } else if (command->cmd == LC_CODE_SIGNATURE &&
                   command->cmdsize >= sizeof(struct linkedit_data_command)) {
            struct linkedit_data_command *signature = (struct linkedit_data_command *)command;
            slice->codeSignatureOffset = signature->dataoff;
            slice->codeSignatureSize = signature->datasize;
        }
        position += command->cmdsize;
    }
    free(commands);
    return 0;
}

static int ps_parse_file(const char *path, PSSlice *slices, int capacity, int *count,
                         char *error, size_t errorLength) {
    *count = 0;
    int fd = open(path, O_RDONLY);
    if (fd < 0) {
        char message[256]; snprintf(message, sizeof(message), "Open failed: %s", strerror(errno));
        ps_set_error(error, errorLength, message);
        return -1;
    }

    struct stat info;
    uint32_t magic = 0;
    if (fstat(fd, &info) != 0 || !ps_pread_all(fd, &magic, sizeof(magic), 0)) {
        close(fd);
        ps_set_error(error, errorLength, "Could not read Mach-O file metadata");
        return -1;
    }

    int result = 0;
    if (magic == MH_MAGIC_64) {
        result = ps_parse_thin(fd, 0, (uint64_t)info.st_size, &slices[0], error, errorLength);
        if (result == 0) *count = 1;
    } else if (magic == FAT_CIGAM || magic == FAT_CIGAM_64) {
        struct fat_header fatHeader;
        if (!ps_pread_all(fd, &fatHeader, sizeof(fatHeader), 0)) {
            result = -1;
            ps_set_error(error, errorLength, "Could not read universal Mach-O header");
        } else {
            bool is64 = (magic == FAT_CIGAM_64 || magic == FAT_MAGIC_64);
            uint32_t architectureCount = OSSwapBigToHostInt32(fatHeader.nfat_arch);
            uint64_t cursor = sizeof(fatHeader);
            for (uint32_t index = 0; index < architectureCount && *count < capacity; index++) {
                uint64_t sliceOffset = 0, sliceSize = 0;
                uint32_t cpuType = 0;
                if (is64) {
                    struct fat_arch_64 arch;
                    if (!ps_pread_all(fd, &arch, sizeof(arch), (off_t)cursor)) break;
                    cursor += sizeof(arch);
                    sliceOffset = OSSwapBigToHostInt64(arch.offset);
                    sliceSize = OSSwapBigToHostInt64(arch.size);
                    cpuType = OSSwapBigToHostInt32(arch.cputype);
                } else {
                    struct fat_arch arch;
                    if (!ps_pread_all(fd, &arch, sizeof(arch), (off_t)cursor)) break;
                    cursor += sizeof(arch);
                    sliceOffset = OSSwapBigToHostInt32(arch.offset);
                    sliceSize = OSSwapBigToHostInt32(arch.size);
                    cpuType = OSSwapBigToHostInt32(arch.cputype);
                }
                if ((cpuType & CPU_ARCH_ABI64) == 0) continue;
                if (sliceOffset > (uint64_t)info.st_size || sliceSize > (uint64_t)info.st_size - sliceOffset) continue;
                char sliceError[256] = {0};
                if (ps_parse_thin(fd, sliceOffset, sliceSize, &slices[*count], sliceError, sizeof(sliceError)) == 0) {
                    (*count)++;
                }
            }
            if (*count == 0) {
                result = -1;
                ps_set_error(error, errorLength, "No supported 64-bit Mach-O slices found");
            }
        }
    } else {
        result = -1;
        ps_set_error(error, errorLength, "File is not a supported Mach-O executable");
    }
    close(fd);
    return result;
}

typedef int (*PSMremapFunction)(void *, size_t, uint32_t, uint32_t, uint32_t);

static int ps_raw_mremap_encrypted(void *address, size_t length, uint32_t cryptID,
                                   uint32_t cpuType, uint32_t cpuSubtype) {
#if defined(__arm64__)
    register uint64_t x0 __asm__("x0") = (uint64_t)(uintptr_t)address;
    register uint64_t x1 __asm__("x1") = (uint64_t)length;
    register uint64_t x2 __asm__("x2") = cryptID;
    register uint64_t x3 __asm__("x3") = cpuType;
    register uint64_t x4 __asm__("x4") = cpuSubtype;
    register uint64_t x16 __asm__("x16") = 489;
    uint64_t failed = 0;
    __asm__ volatile("svc #0x80\n\tcset %w[failed], cs"
                     : "+r"(x0), "+r"(x1), [failed] "=r"(failed)
                     : "r"(x2), "r"(x3), "r"(x4), "r"(x16)
                     : "memory", "cc");
    if (failed) { errno = (int)x0; return -1; }
    return (int)x0;
#else
    (void)address; (void)length; (void)cryptID; (void)cpuType; (void)cpuSubtype;
    errno = ENOTSUP;
    return -1;
#endif
}

static int ps_call_mremap_encrypted(void *address, size_t length, uint32_t cryptID,
                                    uint32_t cpuType, uint32_t cpuSubtype) {
    PSMremapFunction function = (PSMremapFunction)dlsym(RTLD_DEFAULT, "mremap_encrypted");
    if (function) return function(address, length, cryptID, cpuType, cpuSubtype);
    return ps_raw_mremap_encrypted(address, length, cryptID, cpuType, cpuSubtype);
}

static double ps_entropy(const uint8_t *bytes, size_t length) {
    if (!bytes || length == 0) return 0.0;
    uint64_t counts[256] = {0};
    for (size_t index = 0; index < length; index++) counts[bytes[index]]++;
    double entropy = 0.0;
    for (size_t index = 0; index < 256; index++) {
        if (counts[index] == 0) continue;
        double probability = (double)counts[index] / (double)length;
        entropy -= probability * (log(probability) / log(2.0));
    }
    return entropy;
}

static uint32_t ps_code_words(const uint8_t *bytes, size_t length) {
    uint32_t count = 0;
    for (size_t offset = 0; offset + sizeof(uint32_t) <= length; offset += sizeof(uint32_t)) {
        uint32_t word = 0;
        memcpy(&word, bytes + offset, sizeof(word));
        if (word == 0xd65f03c0 || word == 0xd503237f || word == 0xd65f0fff) count++;
    }
    return count;
}

static int ps_try_decrypt_slice(int sourceFD, const PSSlice *slice, bool addCodeSignature,
                                uint8_t *decrypted, char *error, size_t errorLength) {
    const uint64_t pageSize = (uint64_t)getpagesize();
    if (!pageSize || slice->sliceOffset % pageSize || slice->cryptOffset % pageSize) {
        ps_set_error(error, errorLength, "Encrypted Mach-O range is not page aligned");
        return -1;
    }
    if (!slice->cryptSize) {
        ps_set_error(error, errorLength, "Mach-O encryption range is empty");
        return -1;
    }
    uint64_t rangeEnd = (uint64_t)slice->cryptOffset + (uint64_t)slice->cryptSize;
    uint64_t mappedLength = (rangeEnd + pageSize - 1) & ~(pageSize - 1);
    if (mappedLength > slice->sliceSize) {
        ps_set_error(error, errorLength, "Mach-O encryption range exceeds its slice");
        return -1;
    }

    uint8_t *original = (uint8_t *)malloc(slice->cryptSize);
    if (!original) {
        ps_set_error(error, errorLength, "Not enough memory for encrypted code range");
        return -1;
    }
    if (!ps_pread_all(sourceFD, original, slice->cryptSize,
                      (off_t)(slice->sliceOffset + slice->cryptOffset))) {
        free(original);
        ps_set_error(error, errorLength, "Could not read encrypted code range");
        return -1;
    }

#ifdef F_ADDFILESIGS_RETURN
    // Match FairPlayed's optional signature-registration attempt. The first
    // pass deliberately skips this; the second retries with the embedded signature.
    if (addCodeSignature && slice->codeSignatureSize != 0) {
        fsignatures_t signature = {
            .fs_file_start = (off_t)slice->sliceOffset,
            .fs_blob_start = (void *)(uintptr_t)slice->codeSignatureOffset,
            .fs_blob_size = (size_t)slice->codeSignatureSize
        };
        if (fcntl(sourceFD, F_ADDFILESIGS_RETURN, &signature) == -1) {
            char message[256];
            snprintf(message, sizeof(message), "F_ADDFILESIGS_RETURN failed: %s", strerror(errno));
            ps_set_error(error, errorLength, message);
        }
    }
#else
    (void)addCodeSignature;
#endif

    // The iOS 27 FairPlayed path uses CRYPTID_MODEL_ENCRYPTION (2) with
    // PROT_READ only. Avoid PROT_EXEC: that path is unnecessary and can be
    // blocked by executable-memory policy/JIT state in hosted environments.
    void *mapping = mmap(NULL, (size_t)mappedLength, PROT_READ, MAP_PRIVATE,
                         sourceFD, (off_t)slice->sliceOffset);
    if (mapping == MAP_FAILED) {
        char message[256]; snprintf(message, sizeof(message), "mmap failed: %s", strerror(errno));
        ps_set_error(error, errorLength, message);
        free(original);
        return -1;
    }

    errno = 0;
    int remapResult = ps_call_mremap_encrypted((uint8_t *)mapping + slice->cryptOffset,
                                               slice->cryptSize, 2,
                                               slice->cpuType, slice->cpuSubtype);
    int remapError = errno;
    bool allPagesRead = remapResult == 0;
    if (allPagesRead) {
        uint64_t pageCount = (slice->cryptSize + pageSize - 1) / pageSize;
        for (uint64_t index = 0; index < pageCount; index++) {
            uint64_t pageOffset = index * pageSize;
            size_t bytes = (size_t)((pageOffset + pageSize <= slice->cryptSize)
                                    ? pageSize : (slice->cryptSize - pageOffset));
            vm_size_t bytesRead = 0;
            kern_return_t status = vm_read_overwrite(
                mach_task_self(),
                (vm_address_t)(uintptr_t)((uint8_t *)mapping + slice->cryptOffset + pageOffset),
                (vm_size_t)bytes,
                (vm_address_t)(uintptr_t)(decrypted + pageOffset),
                &bytesRead);
            if (status != KERN_SUCCESS || bytesRead != bytes) {
                allPagesRead = false;
                break;
            }
        }
    }

    bool changed = allPagesRead && memcmp(original, decrypted, slice->cryptSize) != 0;
    double entropyBefore = ps_entropy(original, slice->cryptSize);
    double entropyAfter = allPagesRead ? ps_entropy(decrypted, slice->cryptSize) : 8.0;
    uint32_t codeWordsBefore = ps_code_words(original, slice->cryptSize);
    uint32_t codeWordsAfter = allPagesRead ? ps_code_words(decrypted, slice->cryptSize) : 0;
    munmap(mapping, (size_t)mappedLength);
    free(original);

    bool looksDecrypted = remapResult == 0 && allPagesRead && changed &&
                          entropyBefore > 7.9 && entropyAfter < 7.5 &&
                          codeWordsAfter > codeWordsBefore;
    if (!looksDecrypted) {
        char message[384];
        if (remapResult != 0) {
            snprintf(message, sizeof(message), "mremap_encrypted(cryptid=2) failed: %s (errno %d)", strerror(remapError), remapError);
        } else if (!allPagesRead) {
            snprintf(message, sizeof(message), "FairPlay pages could not all be read back");
        } else if (!changed) {
            snprintf(message, sizeof(message), "FairPlay returned unchanged code pages");
        } else {
            snprintf(message, sizeof(message), "Output did not pass decrypted-code validation (entropy %.3f -> %.3f, code words %u -> %u)",
                     entropyBefore, entropyAfter, codeWordsBefore, codeWordsAfter);
        }
        ps_set_error(error, errorLength, message);
        return -1;
    }
    return 0;
}

int32_t PSFairPlayDecryptMachO(const char *sourcePath, const char *outputPath, char *error, size_t errorLength) {
    if (!sourcePath || !*sourcePath || !outputPath || !*outputPath) {
        ps_set_error(error, errorLength, "Mach-O source or output path is empty");
        return -1;
    }

    PSSlice slices[PS_MAX_SLICES];
    int sliceCount = 0;
    if (ps_parse_file(sourcePath, slices, PS_MAX_SLICES, &sliceCount, error, errorLength) != 0) return -1;

    bool containsEncryptedCode = false;
    for (int index = 0; index < sliceCount; index++) {
        if (slices[index].hasEncryptionCommand && slices[index].cryptID != 0 && slices[index].cryptSize != 0) {
            containsEncryptedCode = true;
        }
    }
    if (!containsEncryptedCode) return 1;

    int sourceFD = open(sourcePath, O_RDONLY);
    int outputFD = open(outputPath, O_RDWR);
    if (sourceFD < 0 || outputFD < 0) {
        if (sourceFD >= 0) close(sourceFD);
        if (outputFD >= 0) close(outputFD);
        char message[256]; snprintf(message, sizeof(message), "Could not open source or output executable: %s", strerror(errno));
        ps_set_error(error, errorLength, message);
        return -1;
    }

    int result = 0;
    for (int index = 0; index < sliceCount; index++) {
        PSSlice *slice = &slices[index];
        if (!slice->hasEncryptionCommand || slice->cryptID == 0 || slice->cryptSize == 0) continue;

        uint8_t *decrypted = (uint8_t *)malloc(slice->cryptSize);
        if (!decrypted) {
            ps_set_error(error, errorLength, "Not enough memory to decrypt Mach-O slice");
            result = -1;
            break;
        }

        char attemptError[256] = {0};
        int attempt = ps_try_decrypt_slice(sourceFD, slice, false, decrypted,
                                           attemptError, sizeof(attemptError));
        if (attempt != 0 && slice->codeSignatureSize != 0) {
            memset(attemptError, 0, sizeof(attemptError));
            attempt = ps_try_decrypt_slice(sourceFD, slice, true, decrypted,
                                           attemptError, sizeof(attemptError));
        }

        if (attempt != 0) {
            ps_set_error(error, errorLength, attemptError);
            free(decrypted);
            result = -1;
            break;
        }

        uint32_t clearCryptID = 0;
        off_t dataOffset = (off_t)(slice->sliceOffset + slice->cryptOffset);
        off_t flagOffset = (off_t)(slice->encryptionCommandOffset +
                                  offsetof(struct encryption_info_command, cryptid));
        if (!ps_pwrite_all(outputFD, decrypted, slice->cryptSize, dataOffset) ||
            !ps_pwrite_all(outputFD, &clearCryptID, sizeof(clearCryptID), flagOffset)) {
            char message[256]; snprintf(message, sizeof(message), "Could not write decrypted Mach-O data: %s", strerror(errno));
            ps_set_error(error, errorLength, message);
            result = -1;
        }
        free(decrypted);
        if (result != 0) break;
    }

    fsync(outputFD);
    close(outputFD);
    close(sourceFD);
    return result;
}
