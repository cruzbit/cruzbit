cmake_minimum_required(VERSION 3.12)
project(cuda)
find_package(CUDA REQUIRED)
set(CUDA_SEPARABLE_COMPILATION ON)
set(CUDA_NVCC_FLAGS ${CUDA_NVCC_FLAGS}; -Xcompiler -fPIC)
CUDA_ADD_LIBRARY(cuda SHARED
  mine.cu
  sha3.cu
  )
