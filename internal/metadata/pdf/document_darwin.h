#ifndef HYLLA_PDF_DOCUMENT_H
#define HYLLA_PDF_DOCUMENT_H
#include <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>

// Read only from a duplicate of the checked file descriptor. Do not reopen
// paths or change the original file's seek position.
static size_t coverRead(void *info, void *buffer, off_t position, size_t count) {
    int fd = *(int *)info;
    ssize_t result;
    do { result = pread(fd, buffer, count, position); } while (result < 0 && errno == EINTR);
    return result > 0 ? (size_t)result : 0;
}
static void coverRelease(void *info) {
    close(*(int *)info);
    free(info);
}
static CGPDFDocumentRef openPDFDocument(int sourceFD, off_t size) {
    int *fd = malloc(sizeof(int));
    if (!fd) return NULL;
    *fd = dup(sourceFD);
    if (*fd < 0) { free(fd); return NULL; }
    CGDataProviderDirectCallbacks callbacks = {0, NULL, NULL, coverRead, coverRelease};
    CGDataProviderRef provider = CGDataProviderCreateDirect(fd, size, &callbacks);
    if (!provider) { coverRelease(fd); return NULL; }
    CGPDFDocumentRef document = CGPDFDocumentCreateWithProvider(provider);
    CGDataProviderRelease(provider);
    return document;
}
#endif
