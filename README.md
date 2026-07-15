# robotstxtd

A managed service to evaluate `robots.txt`-related access.

* https://www.rfc-editor.org/rfc/rfc9309.html

## Usage

Server

```
go run ./cmd/robotstxtd serve \
  --bind 127.0.0.1:9078 \
  --bot-identifier=Examplebot \
  --bot-version=v0.1 \
  --bot-url=http://example.com/ \
  --bot-email=bot@example.com \
  --db-sqlite3-dsn=/tmp/robotstxtd.sqlite3
```

Client

```
go run ./cmd/robotstxtd rpc --server 127.0.0.1:9078 \
  evaluate-uri \
  'https://geolocation.onetrust.com/cookieconsentpub/v1/geo/location/dnsfeed'
```

## Development

Build Image

```
podman build -t asdf -f build/docker/robotstxtd/Dockerfile .
```

Rebuild Protocol Buffers

```
cd protobuf
buf generate
```

## License

[MIT License](LICENSE)
