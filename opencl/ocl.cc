// this is partially re-worked from https://github.com/tcatm/oclminer/
// primary changes to this file were renaming and addition of additional
// parameters. -asdvxgxasjab 7/30/19
#include "ocl.h"
#include "sha3.h"
#include <stdint.h>

int num_devices_cl() {
  cl_int status = 0;

  cl_uint num_platforms;
  cl_platform_id platform = NULL;
  status = clGetPlatformIDs(0, NULL, &num_platforms);
  if (status != CL_SUCCESS) {
    printf("Error: Getting Platforms. (clGetPlatformsIDs)\n");
    return -1;
  }

  if (num_platforms > 0) {
    cl_platform_id *platforms =
        (cl_platform_id *)malloc(num_platforms * sizeof(cl_platform_id));
    status = clGetPlatformIDs(num_platforms, platforms, NULL);
    if (status != CL_SUCCESS) {
      printf("Error: Getting Platform Ids. (clGetPlatformsIDs)\n");
      return -1;
    }

    unsigned int i;
    for (i = 0; i < num_platforms; ++i) {
      char pbuff[100];
      status = clGetPlatformInfo(platforms[i], CL_PLATFORM_VENDOR,
                                 sizeof(pbuff), pbuff, NULL);
      if (status != CL_SUCCESS) {
        printf("Error: Getting Platform Info. (clGetPlatformInfo)\n");
        free(platforms);
        return -1;
      }
      platform = platforms[i];
      if (!strcmp(pbuff, "Advanced Micro Devices, Inc.")) {
        break;
      }
    }
    free(platforms);
  }

  if (platform == NULL) {
    perror("NULL platform found!\n");
    return -1;
  }

  cl_uint num_devices;
  status = clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, 0, NULL, &num_devices);
  if (status != CL_SUCCESS) {
    printf("Error: Getting Device IDs (num)\n");
    return -1;
  }

  return num_devices;
}

static char *read_file(const char *filename, int *length) {
  FILE *f = fopen(filename, "r");
  void *buffer;

  if (!f) {
    fprintf(stderr, "Unable to open %s for reading\n", filename);
    return NULL;
  }

  fseek(f, 0, SEEK_END);
  *length = ftell(f);
  fseek(f, 0, SEEK_SET);

  buffer = (char *)malloc(*length + 1);
  *length = fread(buffer, 1, *length, f);
  fclose(f);
  ((char *)buffer)[*length] = '\0';

  return (char *)buffer;
}

cl_state *init_cl(int gpu, char *name, size_t name_len) {
  cl_int status = 0;
  cl_state *device_state = new cl_state;

  cl_uint num_platforms;
  cl_platform_id platform = NULL;
  status = clGetPlatformIDs(0, NULL, &num_platforms);
  if (status != CL_SUCCESS) {
    printf("Error: Getting Platforms. (clGetPlatformsIDs)\n");
    return NULL;
  }

  if (num_platforms > 0) {
    cl_platform_id *platforms =
        (cl_platform_id *)malloc(num_platforms * sizeof(cl_platform_id));
    status = clGetPlatformIDs(num_platforms, platforms, NULL);
    if (status != CL_SUCCESS) {
      printf("Error: Getting Platform Ids. (clGetPlatformsIDs)\n");
      return NULL;
    }

    unsigned int i;
    for (i = 0; i < num_platforms; ++i) {
      char pbuff[100];
      status = clGetPlatformInfo(platforms[i], CL_PLATFORM_VENDOR,
                                 sizeof(pbuff), pbuff, NULL);
      if (status != CL_SUCCESS) {
        printf("Error: Getting Platform Info. (clGetPlatformInfo)\n");
        free(platforms);
        return NULL;
      }
      platform = platforms[i];
      if (!strcmp(pbuff, "Advanced Micro Devices, Inc.")) {
        break;
      }
    }
    free(platforms);
  }

  if (platform == NULL) {
    perror("NULL platform found!\n");
    return NULL;
  }

  cl_uint num_devices;
  status = clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, 0, NULL, &num_devices);
  if (status != CL_SUCCESS) {
    printf("Error: Getting Device IDs (num)\n");
    return NULL;
  }

  cl_device_id *devices;
  if (num_devices > 0) {
    devices = (cl_device_id *)malloc(num_devices * sizeof(cl_device_id));

    /* Now, get the device list data */

    status = clGetDeviceIDs(platform, CL_DEVICE_TYPE_GPU, num_devices, devices,
                            NULL);
    if (status != CL_SUCCESS) {
      printf("Error: Getting Device IDs (list)\n");
      return NULL;
    }

    printf("List of devices:\n");

    int i;
    for (i = 0; i < num_devices; i++) {
      char pbuff[100];
      status = clGetDeviceInfo(devices[i], CL_DEVICE_NAME, sizeof(pbuff), pbuff,
                               NULL);
      if (status != CL_SUCCESS) {
        printf("Error: Getting Device Info\n");
        return NULL;
      }

      printf("\t%i\t%s\n", i, pbuff);
    }

    if (gpu >= 0 && gpu < num_devices) {
      char pbuff[100];
      status = clGetDeviceInfo(devices[gpu], CL_DEVICE_NAME, sizeof(pbuff),
                               pbuff, NULL);
      if (status != CL_SUCCESS) {
        printf("Error: Getting Device Info\n");
        return NULL;
      }

      printf("Selected %i: %s\n", gpu, pbuff);
      strncpy(name, pbuff, name_len);
    } else {
      printf("Invalid GPU %i\n", gpu);
      return NULL;
    }

  } else
    return NULL;

  cl_context_properties cps[3] = {CL_CONTEXT_PLATFORM,
                                  (cl_context_properties)platform, 0};

  device_state->context =
      clCreateContextFromType(cps, CL_DEVICE_TYPE_GPU, NULL, NULL, &status);
  if (status != CL_SUCCESS) {
    printf("Error: Creating Context. (clCreateContextFromType)\n");
    return NULL;
  }

  /////////////////////////////////////////////////////////////////
  // Load CL file, build CL program object, create CL kernel object
  /////////////////////////////////////////////////////////////////
  //
  const char *filename = "cruzbit.cl";
  int pl;
  char *source = read_file(filename, &pl);
  if (source == NULL) {
    return NULL;
  }
  size_t sourceSize[] = {(size_t)pl};

  device_state->program = clCreateProgramWithSource(
      device_state->context, 1, (const char **)&source, sourceSize, &status);
  if (status != CL_SUCCESS) {
    printf(
        "Error: Loading Binary into cl_program (clCreateProgramWithBinary)\n");
    return NULL;
  }

  /* create a cl program executable for all the devices specified */
  status =
      clBuildProgram(device_state->program, 1, &devices[gpu], "", NULL, NULL);
  if (status != CL_SUCCESS) {
    printf("Error: Building Program (clBuildProgram)\n");
    size_t logSize;
    status = clGetProgramBuildInfo(device_state->program, devices[gpu],
                                   CL_PROGRAM_BUILD_LOG, 0, NULL, &logSize);

    char *log = (char *)malloc(logSize);
    status = clGetProgramBuildInfo(device_state->program, devices[gpu],
                                   CL_PROGRAM_BUILD_LOG, logSize, log, NULL);
    printf("%s\n", log);
    return NULL;
  }

  /* get a kernel object handle for a kernel with the given name */
  device_state->kernel =
      clCreateKernel(device_state->program, "cruzbit", &status);
  if (status != CL_SUCCESS) {
    printf("Error: Creating Kernel from program. (clCreateKernel)\n");
    return NULL;
  }

  /////////////////////////////////////////////////////////////////
  // Create an OpenCL command queue
  /////////////////////////////////////////////////////////////////
  device_state->command_queue =
      clCreateCommandQueue(device_state->context, devices[gpu], 0, &status);
  if (status != CL_SUCCESS) {
    printf("Creating Command Queue. (clCreateCommandQueue)\n");
    return NULL;
  }

  device_state->arg0 = clCreateBuffer(device_state->context, CL_MEM_READ_WRITE,
                                      sizeof(miner_state), NULL, &status);
  if (status != CL_SUCCESS) {
    printf("Error: clCreateBuffer (inputBuffer)\n");
    return NULL;
  }

  device_state->arg1 = clCreateBuffer(device_state->context, CL_MEM_READ_WRITE,
                                      sizeof(int64_t), NULL, &status);
  if (status != CL_SUCCESS) {
    printf("Error: clCreateBuffer (foundNonce)\n");
    return NULL;
  }

  device_state->arg2 = clCreateBuffer(device_state->context, CL_MEM_READ_WRITE,
                                      sizeof(uint8_t), NULL, &status);
  if (status != CL_SUCCESS) {
    printf("Error: clCreateBuffer (foundNonce)\n");
    return NULL;
  }

  return device_state;
}
