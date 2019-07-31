cmake_minimum_required (VERSION 3.10)
project(cruzbit_ocl)
find_package(OpenCL REQUIRED)
add_library(cruzbit_ocl SHARED
  ocl.cc
  sha3.cc
  mine.cc
)
target_link_libraries(cruzbit_ocl OpenCL::OpenCL)
install(TARGETS cruzbit_ocl DESTINATION lib)
