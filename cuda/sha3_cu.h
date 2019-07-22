// sha3_cu.h
// 19-Nov-11  Markku-Juhani O. Saarinen <mjos@iki.fi>
// Revised 19-Jul-19 sha3.h to run on Nvidia devices -asdvxgxasjab

#ifndef SHA3_CU_H
#define SHA3_CU_H

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
__device__ void sha3_keccakf_cu(uint64_t st[25]);

// OpenSSL - like interfece
__device__ int sha3_init_cu(sha3_ctx_t *c,
                            int mdlen); // mdlen = hash output in bytes
__device__ int sha3_update_cu(sha3_ctx_t *c, const void *data, size_t len);
__device__ int sha3_final_cu(void *md, sha3_ctx_t *c); // digest goes to md

// compute a sha3 hash (md) of given byte length from "in"
__device__ void *sha3_cu(const void *in, size_t inlen, void *md, int mdlen);

#endif
