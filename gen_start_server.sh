#!/bin/bash
set -xe

workdir=$(pwd)/deps/p5-Server-Starter

docker build -f Dockerfile.fatpack .
image=$(docker build -q -f Dockerfile.fatpack .)

FATPACK_SHEBANG='#! /bin/sh
exec ${START_SERVER_PERL:-perl} -x $0 "$@"
#! perl
'

docker run --rm -v ${workdir}:/work $image fatpack-simple --shebang="$FATPACK_SHEBANG" script/start_server
mv ${workdir}/start_server.fatpack ./start_server
docker rmi $image
