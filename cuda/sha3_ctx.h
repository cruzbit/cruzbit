// sha3_ctx.h
// 19-Nov-11  Markku-Juhani O. Saarinen <mjos@iki.fi>
// Revised 21-Jul-19 moved sha3_ctx_t to its own file -asdvxgxasjab

#ifndef SHA3_CTX_H
#define SHA3_CTX_H

#include <stddef.h>
#include <stdint.h>

// state context
typedef struct {
  union {           // state:
    uint8_t b[200]; // 8-bit bytes
    uint64_t q[25]; // 64-bit words
  } st;
  int pt, rsiz, mdlen; // these don't overflow
} sha3_ctx_t;

#endif
