#include "sha3.h"
#include <cuda.h>
#include <inttypes.h>
#include <stdio.h>
#include <string.h>

static const char *_cudaErrorToString(cudaError_t error) {
  switch (error) {
  case cudaSuccess:
    return "cudaSuccess";

  case cudaErrorMissingConfiguration:
    return "cudaErrorMissingConfiguration";

  case cudaErrorMemoryAllocation:
    return "cudaErrorMemoryAllocation";

  case cudaErrorInitializationError:
    return "cudaErrorInitializationError";

  case cudaErrorLaunchFailure:
    return "cudaErrorLaunchFailure";

  case cudaErrorPriorLaunchFailure:
    return "cudaErrorPriorLaunchFailure";

  case cudaErrorLaunchTimeout:
    return "cudaErrorLaunchTimeout";

  case cudaErrorLaunchOutOfResources:
    return "cudaErrorLaunchOutOfResources";

  case cudaErrorInvalidDeviceFunction:
    return "cudaErrorInvalidDeviceFunction";

  case cudaErrorInvalidConfiguration:
    return "cudaErrorInvalidConfiguration";

  case cudaErrorInvalidDevice:
    return "cudaErrorInvalidDevice";

  case cudaErrorInvalidValue:
    return "cudaErrorInvalidValue";

  case cudaErrorInvalidPitchValue:
    return "cudaErrorInvalidPitchValue";

  case cudaErrorInvalidSymbol:
    return "cudaErrorInvalidSymbol";

  case cudaErrorMapBufferObjectFailed:
    return "cudaErrorMapBufferObjectFailed";

  case cudaErrorUnmapBufferObjectFailed:
    return "cudaErrorUnmapBufferObjectFailed";

  case cudaErrorInvalidHostPointer:
    return "cudaErrorInvalidHostPointer";

  case cudaErrorInvalidDevicePointer:
    return "cudaErrorInvalidDevicePointer";

  case cudaErrorInvalidTexture:
    return "cudaErrorInvalidTexture";

  case cudaErrorInvalidTextureBinding:
    return "cudaErrorInvalidTextureBinding";

  case cudaErrorInvalidChannelDescriptor:
    return "cudaErrorInvalidChannelDescriptor";

  case cudaErrorInvalidMemcpyDirection:
    return "cudaErrorInvalidMemcpyDirection";

  case cudaErrorAddressOfConstant:
    return "cudaErrorAddressOfConstant";

  case cudaErrorTextureFetchFailed:
    return "cudaErrorTextureFetchFailed";

  case cudaErrorTextureNotBound:
    return "cudaErrorTextureNotBound";

  case cudaErrorSynchronizationError:
    return "cudaErrorSynchronizationError";

  case cudaErrorInvalidFilterSetting:
    return "cudaErrorInvalidFilterSetting";

  case cudaErrorInvalidNormSetting:
    return "cudaErrorInvalidNormSetting";

  case cudaErrorMixedDeviceExecution:
    return "cudaErrorMixedDeviceExecution";

  case cudaErrorCudartUnloading:
    return "cudaErrorCudartUnloading";

  case cudaErrorUnknown:
    return "cudaErrorUnknown";

  case cudaErrorNotYetImplemented:
    return "cudaErrorNotYetImplemented";

  case cudaErrorMemoryValueTooLarge:
    return "cudaErrorMemoryValueTooLarge";

  case cudaErrorInvalidResourceHandle:
    return "cudaErrorInvalidResourceHandle";

  case cudaErrorNotReady:
    return "cudaErrorNotReady";

  case cudaErrorInsufficientDriver:
    return "cudaErrorInsufficientDriver";

  case cudaErrorSetOnActiveProcess:
    return "cudaErrorSetOnActiveProcess";

  case cudaErrorInvalidSurface:
    return "cudaErrorInvalidSurface";

  case cudaErrorNoDevice:
    return "cudaErrorNoDevice";

  case cudaErrorECCUncorrectable:
    return "cudaErrorECCUncorrectable";

  case cudaErrorSharedObjectSymbolNotFound:
    return "cudaErrorSharedObjectSymbolNotFound";

  case cudaErrorSharedObjectInitFailed:
    return "cudaErrorSharedObjectInitFailed";

  case cudaErrorUnsupportedLimit:
    return "cudaErrorUnsupportedLimit";

  case cudaErrorDuplicateVariableName:
    return "cudaErrorDuplicateVariableName";

  case cudaErrorDuplicateTextureName:
    return "cudaErrorDuplicateTextureName";

  case cudaErrorDuplicateSurfaceName:
    return "cudaErrorDuplicateSurfaceName";

  case cudaErrorDevicesUnavailable:
    return "cudaErrorDevicesUnavailable";

  case cudaErrorInvalidKernelImage:
    return "cudaErrorInvalidKernelImage";

  case cudaErrorNoKernelImageForDevice:
    return "cudaErrorNoKernelImageForDevice";

  case cudaErrorIncompatibleDriverContext:
    return "cudaErrorIncompatibleDriverContext";

  case cudaErrorPeerAccessAlreadyEnabled:
    return "cudaErrorPeerAccessAlreadyEnabled";

  case cudaErrorPeerAccessNotEnabled:
    return "cudaErrorPeerAccessNotEnabled";

  case cudaErrorDeviceAlreadyInUse:
    return "cudaErrorDeviceAlreadyInUse";

  case cudaErrorProfilerDisabled:
    return "cudaErrorProfilerDisabled";

  case cudaErrorProfilerNotInitialized:
    return "cudaErrorProfilerNotInitialized";

  case cudaErrorProfilerAlreadyStarted:
    return "cudaErrorProfilerAlreadyStarted";

  case cudaErrorProfilerAlreadyStopped:
    return "cudaErrorProfilerAlreadyStopped";

  case cudaErrorAssert:
    return "cudaErrorAssert";

  case cudaErrorTooManyPeers:
    return "cudaErrorTooManyPeers";

  case cudaErrorHostMemoryAlreadyRegistered:
    return "cudaErrorHostMemoryAlreadyRegistered";

  case cudaErrorHostMemoryNotRegistered:
    return "cudaErrorHostMemoryNotRegistered";

  case cudaErrorOperatingSystem:
    return "cudaErrorOperatingSystem";

  case cudaErrorPeerAccessUnsupported:
    return "cudaErrorPeerAccessUnsupported";

  case cudaErrorLaunchMaxDepthExceeded:
    return "cudaErrorLaunchMaxDepthExceeded";

  case cudaErrorLaunchFileScopedTex:
    return "cudaErrorLaunchFileScopedTex";

  case cudaErrorLaunchFileScopedSurf:
    return "cudaErrorLaunchFileScopedSurf";

  case cudaErrorSyncDepthExceeded:
    return "cudaErrorSyncDepthExceeded";

  case cudaErrorLaunchPendingCountExceeded:
    return "cudaErrorLaunchPendingCountExceeded";

  case cudaErrorNotPermitted:
    return "cudaErrorNotPermitted";

  case cudaErrorNotSupported:
    return "cudaErrorNotSupported";

  case cudaErrorHardwareStackError:
    return "cudaErrorHardwareStackError";

  case cudaErrorIllegalInstruction:
    return "cudaErrorIllegalInstruction";

  case cudaErrorMisalignedAddress:
    return "cudaErrorMisalignedAddress";

  case cudaErrorInvalidAddressSpace:
    return "cudaErrorInvalidAddressSpace";

  case cudaErrorInvalidPc:
    return "cudaErrorInvalidPc";

  case cudaErrorIllegalAddress:
    return "cudaErrorIllegalAddress";

  case cudaErrorInvalidPtx:
    return "cudaErrorInvalidPtx";

  case cudaErrorInvalidGraphicsContext:
    return "cudaErrorInvalidGraphicsContext";

  case cudaErrorStartupFailure:
    return "cudaErrorStartupFailure";

  case cudaErrorApiFailureBase:
    return "cudaErrorApiFailureBase";

  case cudaErrorNvlinkUncorrectable:
    return "cudaErrorNvlinkUncorrectable";

  case cudaErrorJitCompilerNotFound:
    return "cudaErrorJitCompilerNotFound";

  case cudaErrorCooperativeLaunchTooLarge:
    return "cudaErrorCooperativeLaunchTooLarge";
  }

  return "<unknown>";
}

__device__ int memcmp_cu(const void *p1, const void *p2, size_t len) {
  for (size_t i = 0; i < len; i++) {
    uint8_t b1 = ((uint8_t *)p1)[i];
    uint8_t b2 = ((uint8_t *)p2)[i];
    if (b1 < b2) {
      return -1;
    }
    if (b1 > b2) {
      return 1;
    }
  }
  return 0;
}

__device__ int strlen_cu(char *s) {
  int i;
  for (i = 0; s[i] != '\0';) {
    i++;
  }
  return i;
}

__device__ char *reverse(char *str) {
  char tmp, *src, *dst;
  size_t len;
  if (str != NULL) {
    len = strlen_cu(str);
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

__device__ int itoa(int64_t n, char s[]) {
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

__device__ void debug_print_buf(const void *buf, size_t len) {
  for (int i = 0; i < len; i++) {
    printf("%c", ((char *)buf)[i]);
  }
  printf("\n");
}

__device__ void debug_print_hash(const void *hash) {
  for (int i = 0; i < 32; i++) {
    printf("%02x", ((char *)hash)[i] & 0xFF);
  }
  printf("\n");
}

// called from the gpu kernel
__global__ void do_sha3(const void *first, size_t first_len, const void *last,
                        size_t last_len, int64_t start_nonce, void *target,
                        int64_t *good_nonce, int *hashes) {
  uint8_t hash[32];
  uint8_t nonce_s[20];

  int index = blockDim.x * blockIdx.x + threadIdx.x;
  int64_t nonce = start_nonce + (int64_t)index;
  size_t n = (size_t)itoa(nonce, (char *)nonce_s);

  sha3_ctx_t sha3;

  sha3_init_cu(&sha3, 32);
  sha3_update_cu(&sha3, first, first_len);
  sha3_update_cu(&sha3, nonce_s, n);
  sha3_update_cu(&sha3, last, last_len);
  sha3_final_cu(hash, &sha3);

  // atomicAdd(hashes, 1);
#if 0
  if (index == 0) {
    debug_print_buf(first, first_len);
    debug_print_buf(nonce_s, n);
    debug_print_buf(last, last_len);
    debug_print_hash(hash);
    debug_print_hash(target);
  }
#endif

  if (memcmp_cu(hash, target, 32) <= 0) {
#if 0
    debug_print_buf(first, first_len);
    debug_print_buf(nonce_s, n);
    debug_print_buf(last, last_len);
    debug_print_hash(target);
    debug_print_hash((char *)hash);
#endif
    // found a solution. not thread-safe but a race is very unlikely
    *good_nonce = nonce;
  }
}

struct miner_state {
  void *first_cu, *last_cu, *target_cu;
  size_t first_len, last_len;
  int num_blocks, block_size, max_threads;
  int64_t *nonce_cu;
  int *hashes_cu;
};

static struct miner_state *states = 0;

extern "C" {

// called on startup
int cuda_init() {
  int device_count = -1;
  cudaError_t error = cudaGetDeviceCount(&device_count);
  if (error != cudaSuccess) {
    printf("cudaGetDeviceCount: %s\n", _cudaErrorToString(error));
    return -1;
  }
  if (device_count <= 0) {
    return -1;
  }

  states = new struct miner_state[device_count];

  for (int i = 0; i < device_count; i++) {
    cudaDeviceProp props;
    error = cudaGetDeviceProperties(&props, i);
    if (error != cudaSuccess) {
      printf("cudaGetDeviceProperties: %s\n", _cudaErrorToString(error));
      return -1;
    }

    states[i].max_threads =
        props.maxThreadsPerMultiProcessor * props.multiProcessorCount;
    states[i].block_size = props.warpSize;
    states[i].num_blocks = states[i].max_threads / states[i].block_size;

    error = cudaSetDevice(i);
    if (error != cudaSuccess) {
      printf("cudaSetDevice: %s\n", _cudaErrorToString(error));
      return -1;
    }

    // allocate memory used on device
    cudaMalloc(&states[i].first_cu, 512);
    cudaMalloc(&states[i].last_cu, 512);
    cudaMalloc(&states[i].target_cu, 32);
    cudaMalloc(&states[i].nonce_cu, sizeof(int64_t));
    cudaMalloc(&states[i].hashes_cu, sizeof(int));

    cudaMemset(states[i].hashes_cu, 0, sizeof(int));
    cudaMemset(states[i].nonce_cu, 0x7F, sizeof(int64_t));
    cudaMemset(states[i].nonce_cu, 0xFF, sizeof(int64_t) - 1);
  }

  return device_count;
}

// called after updating the block header
int miner_update(int miner_num, const void *first, size_t first_len,
                 const void *last, size_t last_len, const void *target) {
  cudaSetDevice(miner_num);

  // copy the first part of the header
  states[miner_num].first_len = first_len;
  cudaMemcpy(states[miner_num].first_cu, first, first_len,
             cudaMemcpyHostToDevice);

  // copy the end part of the header
  states[miner_num].last_len = last_len;
  cudaMemcpy(states[miner_num].last_cu, last, last_len, cudaMemcpyHostToDevice);

  // copy the target
  cudaMemcpy(states[miner_num].target_cu, target, 32, cudaMemcpyHostToDevice);

  // clear nonce
  cudaMemset(states[miner_num].nonce_cu, 0x7F, sizeof(int64_t));
  cudaMemset(states[miner_num].nonce_cu, 0xFF, sizeof(int64_t) - 1);

  return states[miner_num].num_blocks * states[miner_num].block_size;
}

// called in a loop until solved
// returns a solving nonce if found; otherwise 0x7FFFFFFFFFFFFFFF
int64_t miner_mine(int miner_num, int64_t start_nonce) {
  cudaSetDevice(miner_num);
  int64_t nonce;
  cudaMemset(states[miner_num].hashes_cu, 0, sizeof(int));
  int num_blocks = states[miner_num].num_blocks;
  int block_size = states[miner_num].block_size;
  do_sha3<<<num_blocks, block_size>>>(
      states[miner_num].first_cu, states[miner_num].first_len,
      states[miner_num].last_cu, states[miner_num].last_len, start_nonce,
      states[miner_num].target_cu, states[miner_num].nonce_cu,
      states[miner_num].hashes_cu);
  cudaDeviceSynchronize();
  cudaMemcpy(&nonce, states[miner_num].nonce_cu, sizeof(int64_t),
             cudaMemcpyDeviceToHost);
  return nonce;
}
}
