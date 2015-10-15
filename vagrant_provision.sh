#!/bin/bash

apt-get update -qy
apt-get upgrade -qy
apt-get install -qy git

if [ ! -f /usr/local/go/bin/go ]; then
    cd /tmp
    wget -qcO go.tar.gz https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz
    tar -C /usr/local -xzf go.tar.gz
    rm go.tar.gz
fi
export PATH=$PATH:/usr/local/go/bin

mkdir -p /go/src/github.com/danderson
export GOPATH=/go
ln -s /vagrant /go/src/github.com/danderson/pixiecore

cd /go/src/github.com/danderson/pixiecore
go get golang.org/x/crypto/nacl/secretbox
go get golang.org/x/net/ipv4
go build -o /usr/local/bin/pixiecore .

cat > /etc/profile.d/go.sh <<EOF
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/go
EOF

