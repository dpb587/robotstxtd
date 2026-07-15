FROM golang:1.26.4-bookworm AS gobuild

ADD . /mnt/repository/goroot
WORKDIR /mnt/repository/goroot
ENV CGO_ENABLED=1
RUN \
  --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build \
    -o /usr/local/bin/robotstxtd \
    -ldflags='-s -w -extldflags "-static"' \
    ./cmd/robotstxtd

FROM alpine
RUN apk add --no-cache ca-certificates
COPY --from=gobuild /usr/local/bin/robotstxtd /bin/robotstxtd
RUN mkdir /result
WORKDIR /result
ENTRYPOINT ["/bin/robotstxtd"]
CMD ["--help"]
