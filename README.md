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

`--socks5-proxy` is optional. An empty value dials upstream directly. A value looks like `socks5://[user:password@]host:port`. The proxy resolves upstream DNS; this process does not. Each request dials the proxy again, and keep-alives stay off, so a proxy DNS name can round-robin. The password is never logged. `--ca-cert` still supplies extra TLS roots for upstream HTTPS.

`--inspector-bind` is optional and off unless set, for example `127.0.0.1:9079`. The inspector is an unauthenticated site on that address. It calls the in-process service. Bind it to a trusted address.

```
go run ./cmd/robotstxtd serve \
  --bind 127.0.0.1:9078 \
  --inspector-bind 127.0.0.1:9079 \
  --socks5-proxy 'socks5://user:password@proxy.example:1080' \
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
  'https://example.com/page'

go run ./cmd/robotstxtd rpc --server 127.0.0.1:9078 \
  list-files --page-size 50
```

`list-files` pages with `--page-token` (the last generation from the previous page) and `--include-invalidated`.

## Generation and cache

Each stored robots.txt response has a generation: a process-local, zero-padded 19-digit Unix nanosecond counter. A mutex bumps the value when the clock does not move, including within the same nanosecond, so every stored generation is strictly greater than the previous one. The decimal strings sort in the same order as the numbers.

The SQLite table is `cache`. One row is active per `service_uri` (`invalidate_time IS NULL`). An index on `generation` serves keyset pages. This is a hard cutover: delete any existing database file before starting. The server does not migrate older tables or `cache_key` values.

## Development

Build the image from `deploy/docker/robotstxtd/Dockerfile`. The build downloads modules; there is no `vendor/` directory.

```
podman build -t robotstxtd -f deploy/docker/robotstxtd/Dockerfile .
```

Rebuild protocol buffers

```
cd protobuf
buf generate
```

## License

[MIT License](LICENSE)
