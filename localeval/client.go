package localeval

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/dpb587/robotstxtd/api"
	"github.com/dpb587/robotstxtd/api/apiutil"
	"github.com/dpb587/robotstxtd/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Client struct {
	upstream               api.ServiceClient
	cacheGetBot            *api.GetBot_Response
	cacheGetBotMutex       sync.Mutex
	cacheGetOriginMutex    sync.Mutex
	cacheGetOriginByOrigin map[string]*api.GetOrigin_Response
}

var _ api.ServiceClient = &Client{}

func NewClient(upstream api.ServiceClient) *Client {
	return &Client{
		upstream:               upstream,
		cacheGetOriginByOrigin: map[string]*api.GetOrigin_Response{},
	}
}

func (c *Client) GetBot(ctx context.Context, req *api.GetBot_Request, opts ...grpc.CallOption) (*api.GetBot_Response, error) {
	c.cacheGetBotMutex.Lock()
	if c.cacheGetBot == nil {
		res, err := c.upstream.GetBot(ctx, req, opts...)
		if err != nil {
			c.cacheGetBotMutex.Unlock()

			return nil, err
		}

		c.cacheGetBot = res
	}

	c.cacheGetBotMutex.Unlock()

	return c.cacheGetBot, nil
}

func (c *Client) GetOrigin(ctx context.Context, req *api.GetOrigin_Request, opts ...grpc.CallOption) (*api.GetOrigin_Response, error) {
	c.cacheGetOriginMutex.Lock()

	cached, known := c.cacheGetOriginByOrigin[req.Origin]
	if known {
		if cached.StaleTime.AsTime().After(time.Now()) {
			c.cacheGetOriginMutex.Unlock()

			return cached, nil
		}

		delete(c.cacheGetOriginByOrigin, req.Origin)
	}

	res, err := c.upstream.GetOrigin(ctx, req, opts...)
	if err != nil {
		c.cacheGetOriginMutex.Unlock()

		return nil, err
	}

	c.cacheGetOriginByOrigin[req.Origin] = res

	c.cacheGetOriginMutex.Unlock()

	return res, nil
}

func (c *Client) EvalUrl(ctx context.Context, req *api.EvalUrl_Request, opts ...grpc.CallOption) (*api.EvalUrl_Response, error) {
	bot, err := c.GetBot(ctx, &api.GetBot_Request{})
	if err != nil {
		return nil, fmt.Errorf("bot: %v", err)
	}

	parsedURL, err := url.Parse(req.Url)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "url could not be parsed")
	}

	res, err := c.GetOrigin(ctx, &api.GetOrigin_Request{
		Origin: internal.Origin(parsedURL),
	})
	if err != nil {
		return nil, err
	}

	return apiutil.Evaluate(res, bot.Bot, req.Url, true), nil
}

func (c *Client) FlushOrigin(ctx context.Context, req *api.FlushOrigin_Request, opts ...grpc.CallOption) (*api.FlushOrigin_Response, error) {
	delete(c.cacheGetOriginByOrigin, req.Origin)

	return c.upstream.FlushOrigin(ctx, req, opts...)
}
