package proxyrpcctx

import "context"

// TODO deprecate; used as workaround for non-proxyrpc-aware middleware, but seems risky for the value to be ignored in
// a misconfigured setup which expects/assumes the ip to be used later

type upstreamOriginIpValueKey struct{}

var upstreamOriginIpValue upstreamOriginIpValueKey

func WithUpstreamOriginIp(ctx context.Context, upstreamOriginIp string) context.Context {
	return context.WithValue(ctx, upstreamOriginIpValue, upstreamOriginIp)
}

func UpstreamOriginIp(ctx context.Context) (string, bool) {
	upstreamOriginIp, ok := ctx.Value(upstreamOriginIpValue).(string)
	return upstreamOriginIp, ok
}
