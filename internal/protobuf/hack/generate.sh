#!/bin/bash

set -eou pipefail

datapact_dir="$( cd "$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )/../../.." ; pwd )"

module="$( grep ^module go.mod | awk '{ print $2 }')"

export PROTOTMP="${datapact_dir}/tmp/protobuf"
export PATH="${PROTOTMP}/bin:${PATH}"

if ! [ -e "${PROTOTMP}/bin/protoc" ]
then
  mkdir -p "${PROTOTMP}"
  pushd "${PROTOTMP}"
  curl -Lo archive.zip https://github.com/protocolbuffers/protobuf/releases/download/v28.3/protoc-28.3-osx-x86_64.zip
  unzip archive.zip
  rm archive.zip
  popd
fi

for cmd in \
  google.golang.org/protobuf/cmd/protoc-gen-go \
  google.golang.org/grpc/cmd/protoc-gen-go-grpc
do
  GOBIN="${PROTOTMP}/bin" go install "${cmd}"
done

#
# ready
#

if [[ "${1:-}" == "delete" ]]
then
  find ./ \
    \( -name '*.pb.go' -o -name '*.gw.go' \) \
    -delete
fi

while read -r schemaprotodir
do
  protoc -I . \
    -I ./internal/protobuf/schemaprotoimport \
    -I ../../.. \
    --go_out ./ \
      --go_opt module="${module}" \
    --go-grpc_out ./ \
      --go-grpc_opt module="${module}" \
    $( find "${schemaprotodir}" -name '*.proto' )
done < <(
  find ./ -type d -name schemaproto \
    | sort
)
