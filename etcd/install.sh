pwd=$(pwd)
mkdir -p $GOPATH/src/github.com/coreos
cd $GOPATH/src/github.com/coreos
git clone https://github.com/coreos/etcd.git
cd etcd
./build
mkdir -p $GOPATH/bin
mv bin/etcd $GOPATH/bin
cd $pwd
