#include "ocl.h"
#include "sha3.h"
#include <stdint.h>

cl_state *cl_states[16];
miner_state *miner_states[16];

// TODO: make this configurable
size_t num_threads = 1 << 20;

extern "C" {

int ocl_init() {
  int device_count = num_devices_cl();

  char name[32];
  for (int i = 0; i < device_count; i++) {
    cl_states[i] = init_cl(i, name, sizeof(name));
    if (cl_states[i] == NULL) {
      return -1;
    }
    miner_states[i] = new miner_state;
  }

  return device_count;
}

int miner_update(int miner_num, const void *first, size_t first_len,
                 const void *last, size_t last_len, const void *target) {
  miner_state *state = miner_states[miner_num];

  // hash the first (largest) part of the header once
  sha3_init(&state->prev_sha3, 32);
  sha3_update(&state->prev_sha3, first, first_len);

  // copy the end part of the header
  state->last_len = last_len;
  memcpy(state->last, last, last_len);

  // copy the target
  memcpy(state->target, target, 32);

  return num_threads;
}

int64_t miner_mine(int miner_num, int64_t start_nonce) {
  cl_state *device_state = cl_states[miner_num];
  int64_t nonce = 0x7FFFFFFFFFFFFFFF;
  size_t global_threads[1];
  size_t local_threads[1];

  global_threads[0] = num_threads;
  local_threads[0] = 256;

  cl_int status = clSetKernelArg(device_state->kernel, 0, sizeof(cl_mem),
                                 (void *)&device_state->arg0);
  if (status != CL_SUCCESS) {
    printf("Error: Setting kernel argument 1.\n");
    return nonce;
  }

  status = clSetKernelArg(device_state->kernel, 1, sizeof(cl_mem),
                          (void *)&device_state->arg1);
  if (status != CL_SUCCESS) {
    printf("Error: Setting kernel argument 2.\n");
    return nonce;
  }

  status = clSetKernelArg(device_state->kernel, 2, sizeof(cl_mem),
                          (void *)&device_state->arg2);
  if (status != CL_SUCCESS) {
    printf("Error: Setting kernel argument 2.\n");
    return nonce;
  }

  status = clEnqueueWriteBuffer(device_state->command_queue, device_state->arg0,
                                CL_TRUE, 0, sizeof(miner_state),
                                (void *)miner_states[miner_num], 0, NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: clEnqueueWriteBuffer failed.\n");
    return nonce;
  }

  status = clEnqueueWriteBuffer(device_state->command_queue, device_state->arg1,
                                CL_TRUE, 0, sizeof(int64_t),
                                (void *)&start_nonce, 0, NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: clEnqueueWriteBuffer failed.\n");
    return nonce;
  }

  uint8_t found = 0;
  status = clEnqueueWriteBuffer(device_state->command_queue, device_state->arg2,
                                CL_TRUE, 0, sizeof(uint8_t), (void *)&found, 0,
                                NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: clEnqueueWriteBuffer failed.\n");
    return nonce;
  }

  clFinish(device_state->command_queue);

  status = clEnqueueNDRangeKernel(device_state->command_queue,
                                  device_state->kernel, 1, NULL, global_threads,
                                  local_threads, 0, NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: Enqueueing kernel onto command queue. "
           "(clEnqueueNDRangeKernel)\n");
    return nonce;
  }

  clFlush(device_state->command_queue);

  status =
      clEnqueueReadBuffer(device_state->command_queue, device_state->arg1,
                          CL_TRUE, 0, sizeof(int64_t), &nonce, 0, NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: clEnqueueReadBuffer failed. (clEnqueueReadBuffer)\n");
    return nonce;
  }

  status =
      clEnqueueReadBuffer(device_state->command_queue, device_state->arg2,
                          CL_TRUE, 0, sizeof(uint8_t), &found, 0, NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: clEnqueueReadBuffer failed. (clEnqueueReadBuffer)\n");
    return nonce;
  }

  if (found == 1) {
    return nonce;
  }

  // not found
  return 0x7FFFFFFFFFFFFFFF;
}
}
