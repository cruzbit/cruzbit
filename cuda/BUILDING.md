## How to build with CUDA support

Tested on Ubuntu Linux 18.10. This assumes you've followed [these steps](https://gist.github.com/setanimals/f562ed7dd1c69af3fbe960c7b9502615) so far.

```bash
sudo apt install -y build-essential
sudo apt install -y cmake
wget https://developer.nvidia.com/compute/cuda/10.1/Prod/local_installers/cuda_10.1.168_418.67_linux.run
sudo sh cuda_10.1.168_418.67_linux.run
rm -rf go/src/github.com/cruzbit/
mkdir -p go/src/github.com/cruzbit/
cd go/src/github.com/cruzbit/
git clone https://github.com/cruzbit/cruzbit.git
cd cruzbit/cuda
mkdir build
cd build
cmake ..
make
sudo make install
cd ../../client
go install -tags cuda
LD_LIBRARY_PATH=/usr/local/lib client -numminers <num GPUs> <whatever other arguments you run your client with>
```
