package client

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/dpb587/httpcached/client/proxyrpcctx"
	"github.com/dpb587/httpcached/internal"
)

type Service struct {
	log             *slog.Logger
	serverTransport *http.Transport
	serverURL       *url.URL
	serverProxyURL  string
	blobClient      *seaweedClient
	tier            string
}

var _ http.RoundTripper = (*Service)(nil)

func NewService(log *slog.Logger, serverTransport *http.Transport, serverURL *url.URL, blobTransport *http.Transport, tier string) *Service {
	if serverTransport == nil {
		serverTransport = &http.Transport{}
	}

	if blobTransport == nil {
		blobTransport = &http.Transport{}
	}

	return &Service{
		log:             log,
		serverURL:       serverURL,
		serverProxyURL:  serverURL.ResolveReference(&url.URL{Path: "/proxy"}).String(),
		serverTransport: serverTransport,
		blobClient:      NewSeaweedClient(log, blobTransport),
		tier:            tier,
	}
}

func (s *Service) RoundTrip(req *http.Request) (*http.Response, error) {
	opts := DoOptions{}

	if v, ok := proxyrpcctx.UpstreamOriginIp(req.Context()); ok {
		opts.UpstreamOriginIp = v
	}

	return s.Do(req, opts)
}

type DoOptions struct {
	// Must be a completely trusted resolution of origin hostname. Proxy server only uses it for connections to upstream,
	// if needed, and continues to rely on Request URI for caching/resolving directives.
	//
	// Use case: clients pre-resolve DNS name to ensure it is allowed (e.g. non-local addresses), and want to ensure
	// the proxy server uses that exact same address (vs resolving it again to a potentially different, unvalidated
	// address (from a malicious DNS server)).
	UpstreamOriginIp string
}

func (s *Service) Do(req *http.Request, opts DoOptions) (*http.Response, error) {
	defer func() {
		if req.Body != nil {
			req.Body.Close()
		}
	}()

	wrappedRequest, err := http.NewRequestWithContext(req.Context(), http.MethodPost, s.serverProxyURL, req.Body)
	if err != nil {
		return nil, fmt.Errorf("proxyrpc: create request: %v", err)
	}

	wrappedRequest.Header.Set("Content-Type", "application/http;msgtype=request")
	wrappedRequest.Header.Set("Proxy-Profile", s.tier)
	wrappedRequest.Header.Set("Upstream-Origin", req.URL.Scheme+"://"+req.URL.Host)

	if len(opts.UpstreamOriginIp) > 0 {
		wrappedRequest.Header.Set("Upstream-Origin-Ip", opts.UpstreamOriginIp)
	}

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())
	err = req.Header.Write(buf)
	if err != nil {
		return nil, fmt.Errorf("proxyrpc: prepare request: %v", err)
	}

	buf.WriteString("\r\n")

	if req.Body == nil {
		wrappedRequest.Header.Set("Content-Length", strconv.FormatInt(int64(buf.Len()), 10))
		wrappedRequest.Body = io.NopCloser(buf)
	} else {
		if contentLength := req.Header.Get("Content-Length"); contentLength != "" {
			contentLengthInt64, err := strconv.ParseInt(contentLength, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("proxyrpc: parse content length: %v", err)
			}

			wrappedRequest.Header.Set("Content-Length", strconv.FormatInt(contentLengthInt64+int64(buf.Len()), 10))
		}

		wrappedRequest.Body = io.NopCloser(io.MultiReader(buf, req.Body))
	}

	res, err := s.serverTransport.RoundTrip(wrappedRequest)
	if err != nil {
		s.log.Error(
			"proxyrpc: transport error",
			"error", err,
		)

		return nil, ServiceError{Err: err}
	} else if res.Body == nil {
		err = errors.New("missing response body")

		s.log.Error(
			"proxyrpc: unexpected response",
			"error", err,
		)

		return nil, ServiceError{Err: err}
	}

	var closeBody = true

	defer func() {
		if closeBody {
			// fmt.Fprintf(os.Stderr, ">> Closed-RPC (%s)\n", req.URL.String())
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
	}()

	if res.StatusCode == http.StatusBadGateway {
		buf, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("proxyrpc: read response: %v", err)
		}

		return nil, fmt.Errorf("proxyrpc: %w", BadGatewayError{s: string(buf)})
	} else if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("unexpected status code: %d", res.StatusCode)

		s.log.Error(
			"proxyrpc: unexpected response",
			"error", err,
		)

		return nil, ServiceError{Err: err}
	}

	unwrappedResponse, err := http.ReadResponse(bufio.NewReader(res.Body), req)
	if err != nil {
		s.log.Error(
			"proxyrpc: failed to unwrap response",
			"error", err,
		)

		return nil, ServiceError{Err: fmt.Errorf("unwrap response: %v", err)}
	}

	if contentLocation := res.Header.Get("Response-Entity-Location"); len(contentLocation) > 0 {
		rc, err := s.blobClient.GetObject(req.Context(), contentLocation)
		if err != nil {
			s.log.Error(
				"proxyrpc: failed to get blob",
				"error", err,
			)

			return nil, ServiceError{Err: fmt.Errorf("get blob: %v", err)}
		}

		unwrappedResponse.Body = rc
	} else if unwrappedResponse.Body == http.NoBody {
		// quick close
	} else if unwrappedResponse.Header.Get("Content-Length") == "0" {
		// quick close
	} else if unwrappedResponse.StatusCode == http.StatusNoContent {
		// quick close
	} else {
		closeBody = false
		unwrappedResponse.Body = internal.NewReaderCloser(unwrappedResponse.Body, unwrappedResponse.Body.Close, func() error {
			// fmt.Fprintf(os.Stderr, ">> Closed-Caller (%s)\n", req.URL.String())

			io.Copy(io.Discard, res.Body)

			return res.Body.Close()
		})
	}

	return unwrappedResponse, nil
}
