
## License

This library either borrows code directly from or was inspired by the following:

* [tiny_sha3](https://github.com/mjosaarinen/tiny_sha3/)
* [oclminer](https://github.com/tcatm/oclminer/)

The [LICENSE](https://github.com/cruzbit/cruzbit/blob/master/opencl/LICENSE) file in this directory is the license associated with tiny_sha3.

The [COPYING](https://github.com/cruzbit/cruzbit/blob/master/opencl/COPYING) file in this directory is the license associated with oclminer.

## How to build with OpenCL support

Tested on Ubuntu Linux 18.04 LTS. This assumes you've followed [these steps](https://gist.github.com/setanimals/f562ed7dd1c69af3fbe960c7b9502615) so far.

```bash
sudo apt install -y build-essential
sudo apt install -y cmake
sudo apt install ocl-icd-* opencl-headers
rm -rf go/src/github.com/cruzbit/
mkdir -p go/src/github.com/cruzbit/
cd go/src/github.com/cruzbit/
git clone https://github.com/cruzbit/cruzbit.git
cd cruzbit/opencl
cp cruzbit.cl ../client/
mkdir build
cd build
cmake ..
make
sudo make install
cd ../../client
go build -tags opencl
LD_LIBRARY_PATH=/usr/local/lib ./client -numminers <num GPUs> <...>
```

Note that the `cruzbit.cl` file must be copied to wherever your `client` binary is.

Further note, to use OpenCL support with Nvidia GPUs you'll need to install the [CUDA Toolkit](https://developer.nvidia.com/cuda-toolkit).
