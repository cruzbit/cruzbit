// this runs on the device

typedef uchar uint8_t;
typedef ulong uint64_t;
typedef long int64_t;

// state context
typedef struct {
  union {           // state:
    uint8_t b[200]; // 8-bit bytes
    uint64_t q[25]; // 64-bit words
  } st;
  int pt, rsiz, mdlen; // these don't overflow
} sha3_ctx_t;

#define WGS __attribute__((reqd_work_group_size(256, 1, 1)))
#define KECCAKF_ROUNDS 24
#define ROTL64(x, y) (((x) << (y)) | ((x) >> (64 - (y))))

void sha3_keccakf(uint64_t st[25]);
int sha3_init(sha3_ctx_t *c, int mdlen);
int sha3_update(sha3_ctx_t *c, __constant void *data, size_t len);
int sha3_update_private(sha3_ctx_t *c, __private void *data, size_t len);
int sha3_final(void *md, sha3_ctx_t *c);
char *reverse(char *str);
int strlen(char *s);
int itoa(int64_t n, char *s);
int memcmp(const void *p1, __constant void *p2, size_t len);
void memcpy(void *dst, __constant void *src, size_t len);
void debug_print_buf(const void *buf, size_t len);
void debug_print_hash(const void *hash);

// update the state with given number of rounds

void sha3_keccakf(uint64_t st[25]) {
  // constants
  const uint64_t keccakf_rndc[24] = {
      0x0000000000000001, 0x0000000000008082, 0x800000000000808a,
      0x8000000080008000, 0x000000000000808b, 0x0000000080000001,
      0x8000000080008081, 0x8000000000008009, 0x000000000000008a,
      0x0000000000000088, 0x0000000080008009, 0x000000008000000a,
      0x000000008000808b, 0x800000000000008b, 0x8000000000008089,
      0x8000000000008003, 0x8000000000008002, 0x8000000000000080,
      0x000000000000800a, 0x800000008000000a, 0x8000000080008081,
      0x8000000000008080, 0x0000000080000001, 0x8000000080008008};
  const int keccakf_rotc[24] = {1,  3,  6,  10, 15, 21, 28, 36, 45, 55, 2,  14,
                                27, 41, 56, 8,  25, 43, 62, 18, 39, 61, 20, 44};
  const int keccakf_piln[24] = {10, 7,  11, 17, 18, 3, 5,  16, 8,  21, 24, 4,
                                15, 23, 19, 13, 12, 2, 20, 14, 22, 9,  6,  1};

  // variables
  int i, j, r;
  uint64_t t, bc[5];

#if __BYTE_ORDER__ != __ORDER_LITTLE_ENDIAN__
  uint8_t *v;

  // endianess conversion. this is redundant on little-endian targets
  for (i = 0; i < 25; i++) {
    v = (uint8_t *)&st[i];
    st[i] = ((uint64_t)v[0]) | (((uint64_t)v[1]) << 8) |
            (((uint64_t)v[2]) << 16) | (((uint64_t)v[3]) << 24) |
            (((uint64_t)v[4]) << 32) | (((uint64_t)v[5]) << 40) |
            (((uint64_t)v[6]) << 48) | (((uint64_t)v[7]) << 56);
  }
#endif

  // actual iteration
  for (r = 0; r < KECCAKF_ROUNDS; r++) {

    // Theta
    for (i = 0; i < 5; i++)
      bc[i] = st[i] ^ st[i + 5] ^ st[i + 10] ^ st[i + 15] ^ st[i + 20];

    for (i = 0; i < 5; i++) {
      t = bc[(i + 4) % 5] ^ ROTL64(bc[(i + 1) % 5], 1);
      for (j = 0; j < 25; j += 5)
        st[j + i] ^= t;
    }

    // Rho Pi
    t = st[1];
    for (i = 0; i < 24; i++) {
      j = keccakf_piln[i];
      bc[0] = st[j];
      st[j] = ROTL64(t, keccakf_rotc[i]);
      t = bc[0];
    }

    //  Chi
    for (j = 0; j < 25; j += 5) {
      for (i = 0; i < 5; i++)
        bc[i] = st[j + i];
      for (i = 0; i < 5; i++)
        st[j + i] ^= (~bc[(i + 1) % 5]) & bc[(i + 2) % 5];
    }

    //  Iota
    st[0] ^= keccakf_rndc[r];
  }

#if __BYTE_ORDER__ != __ORDER_LITTLE_ENDIAN__
  // endianess conversion. this is redundant on little-endian targets
  for (i = 0; i < 25; i++) {
    v = (uint8_t *)&st[i];
    t = st[i];
    v[0] = t & 0xFF;
    v[1] = (t >> 8) & 0xFF;
    v[2] = (t >> 16) & 0xFF;
    v[3] = (t >> 24) & 0xFF;
    v[4] = (t >> 32) & 0xFF;
    v[5] = (t >> 40) & 0xFF;
    v[6] = (t >> 48) & 0xFF;
    v[7] = (t >> 56) & 0xFF;
  }
#endif
}

// Initialize the context for SHA3

int sha3_init(sha3_ctx_t *c, int mdlen) {
  int i;

  for (i = 0; i < 25; i++)
    c->st.q[i] = 0;
  c->mdlen = mdlen;
  c->rsiz = 200 - 2 * mdlen;
  c->pt = 0;

  return 1;
}

// update state with more data

int sha3_update(sha3_ctx_t *c, __constant void *data, size_t len) {
  size_t i;
  int j;

  j = c->pt;
  for (i = 0; i < len; i++) {
    c->st.b[j++] ^= ((__constant uint8_t *)data)[i];
    if (j >= c->rsiz) {
      sha3_keccakf(c->st.q);
      j = 0;
    }
  }
  c->pt = j;

  return 1;
}

int sha3_update_private(sha3_ctx_t *c, __private void *data, size_t len) {
  size_t i;
  int j;

  j = c->pt;
  for (i = 0; i < len; i++) {
    c->st.b[j++] ^= ((__private uint8_t *)data)[i];
    if (j >= c->rsiz) {
      sha3_keccakf(c->st.q);
      j = 0;
    }
  }
  c->pt = j;

  return 1;
}

// finalize and output a hash

int sha3_final(void *md, sha3_ctx_t *c) {
  int i;

  c->st.b[c->pt] ^= 0x06;
  c->st.b[c->rsiz - 1] ^= 0x80;
  sha3_keccakf(c->st.q);

  for (i = 0; i < c->mdlen; i++) {
    ((uint8_t *)md)[i] = c->st.b[i];
  }

  return 1;
}

int memcmp(const void *p1, __constant void *p2, size_t len) {
  for (size_t i = 0; i < len; i++) {
    uint8_t b1 = ((uint8_t *)p1)[i];
    uint8_t b2 = ((__constant uint8_t *)p2)[i];
    if (b1 < b2) {
      return -1;
    }
    if (b1 > b2) {
      return 1;
    }
  }
  return 0;
}

void memcpy(void *dst, __constant void *src, size_t len) {
  for (size_t i = 0; i < len; i++) {
    ((uint8_t *)dst)[i] = ((__constant uint8_t *)src)[i];
  }
}

char *reverse(char *str) {
  char tmp, *src, *dst;
  size_t len;
  if (str != 0) {
    len = strlen(str);
    if (len > 1) {
      src = str;
      dst = src + len - 1;
      while (src < dst) {
        tmp = *src;
        *src++ = *dst;
        *dst-- = tmp;
      }
    }
  }
  return str;
}

int strlen(char *s) {
  int i;
  for (i = 0; s[i] != '\0';) {
    i++;
  }
  return i;
}

int itoa(int64_t n, char *s) {
  int i;
  int64_t sign;

  if ((sign = n) < 0) /* record sign */
    n = -n;           /* make n positive */
  i = 0;

  do {                     /* generate digits in reverse order */
    s[i++] = n % 10 + '0'; /* get next digit */
  } while ((n /= 10) > 0); /* delete it */

  if (sign < 0)
    s[i++] = '-';

  s[i] = '\0';
  reverse(s);
  return i;
}

void debug_print_buf(const void *buf, size_t len) {
  for (int i = 0; i < len; i++) {
    printf("%c", ((char *)buf)[i]);
  }
  printf("\n");
}

void debug_print_hash(const void *hash) {
  for (int i = 0; i < 32; i++) {
    printf("%02x", ((char *)hash)[i] & 0xFF);
  }
  printf("\n");
}

typedef struct {
  sha3_ctx_t prev_sha3;
  uint8_t last[512], target[32];
  size_t last_len;
} miner_state;

__kernel __attribute__((vec_type_hint(uint))) WGS void
cruzbit(__constant miner_state *state, __global int64_t *good_nonce,
        __global uint8_t *found) {
  uint8_t hash[32];
  char nonce_s[20];

  int index = get_global_id(0);
  int64_t nonce = *good_nonce + (int64_t)index;
  size_t n = (size_t)itoa(nonce, nonce_s);

  sha3_ctx_t sha3;
  memcpy(&sha3, (__constant void *)&state->prev_sha3, sizeof(sha3_ctx_t));
  sha3_update_private(&sha3, nonce_s, n);
  sha3_update(&sha3, (__constant char *)state->last, state->last_len);
  sha3_final(hash, &sha3);

  if (memcmp(hash, state->target, 32) <= 0) {
    // found a solution. not thread-safe but a race is very unlikely
    *good_nonce = nonce;
    *found = 1;
  }
}
