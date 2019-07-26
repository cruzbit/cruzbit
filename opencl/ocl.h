// this is partially re-worked from https://github.com/tcatm/oclminer/
// primary changes to this file were renaming and addition of additional
// parameters. -asdvxgxasjab 7/30/19
#ifndef OCL_H_
#define OCL_H_

#define CL_SILENCE_DEPRECATION 1
#ifdef __APPLE_CC__
#include <OpenCL/opencl.h>
#else
#include <CL/cl.h>
#endif
#include "sha3.h"
#include <cstdlib>
#include <stdio.h>
#include <string.h>

typedef struct {
  cl_context context;
  cl_kernel kernel;
  cl_command_queue command_queue;
  cl_program program;
  cl_mem arg0;
  cl_mem arg1;
  cl_mem arg2;
} cl_state;

typedef struct {
  sha3_ctx_t prev_sha3;
  uint8_t last[512], target[32];
  size_t last_len;
} miner_state;

int num_devices_cl();
cl_state *init_cl(int gpu, char *name, size_t name_len);

#endif
