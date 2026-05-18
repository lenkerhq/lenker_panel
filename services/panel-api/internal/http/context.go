package httpapi

import "context"

type requestMetaKey struct{}

// RequestMeta holds IP and User-Agent from the HTTP request.
type RequestMeta struct {
	IP        string
	UserAgent string
}

// SetRequestMeta stores request metadata in context.
func SetRequestMeta(ctx context.Context, ip, userAgent string) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, RequestMeta{IP: ip, UserAgent: userAgent})
}

// GetRequestMeta retrieves request metadata from context.
func GetRequestMeta(ctx context.Context) (string, string) {
	meta, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return meta.IP, meta.UserAgent
}
