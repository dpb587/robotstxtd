FROM golang:1.23-bullseye AS gobuild

ADD . /mnt/repository/goroot
WORKDIR /mnt/repository/goroot
ENV CGO_ENABLED=1
RUN go build \
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