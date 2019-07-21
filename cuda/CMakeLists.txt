cmake_minimum_required(VERSION 3.10)
project(cruzbit_cuda)
find_package(CUDA REQUIRED)
set(CUDA_SEPARABLE_COMPILATION ON)
set(CUDA_NVCC_FLAGS ${CUDA_NVCC_FLAGS}; -Xcompiler -fPIC)
CUDA_ADD_LIBRARY(cruzbit_cuda SHARED
  mine.cu
  sha3.cu
)
install(TARGETS cruzbit_cuda DESTINATION lib)
