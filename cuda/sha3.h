// sha3.h
// 19-Nov-11  Markku-Juhani O. Saarinen <mjos@iki.fi>
// Revised 21-Jul-19 to strip unneeded code and move sha3_ctx_t to its own file.
// we need to build both a host and device version of this code. -asdvxgxasjab

#ifndef SHA3_H
#define SHA3_H

#include "sha3_ctx.h"
#include <stddef.h>
#include <stdint.h>

#ifndef KECCAKF_ROUNDS
#define KECCAKF_ROUNDS 24
#endif

#ifndef ROTL64
#define ROTL64(x, y) (((x) << (y)) | ((x) >> (64 - (y))))
#endif

// Compression function.
void sha3_keccakf(uint64_t st[25]);

// OpenSSL - like interfece
int sha3_init(sha3_ctx_t *c,
              int mdlen); // mdlen = hash output in bytes
int sha3_update(sha3_ctx_t *c, const void *data, size_t len);
int sha3_final(void *md, sha3_ctx_t *c); // digest goes to md

// compute a sha3 hash (md) of given byte length from "in"
void *sha3(const void *in, size_t inlen, void *md, int mdlen);

#endif
