package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
)

type Handler interface {
	RegisterRoutes(mux *http.ServeMux)
}

type RouterDeps struct {
	Logger        *slog.Logger
	Auth          Handler
	Admins        Handler
	Users         Handler
	Plans         Handler
	Subscriptions Handler
	Nodes         Handler
	Audit         Handler
	Devices       Handler
	Traffic       Handler
	Routing       Handler
	Settings      Handler
	Warp          Handler
	Profiles      Handler
	Templates     Handler
	Accounts      Handler
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", Healthz)

	for _, handler := range []Handler{
		deps.Auth,
		deps.Admins,
		deps.Users,
		deps.Plans,
		deps.Subscriptions,
		deps.Nodes,
		deps.Audit,
		deps.Devices,
		deps.Traffic,
		deps.Routing,
		deps.Settings,
		deps.Warp,
		deps.Profiles,
		deps.Templates,
		deps.Accounts,
	} {
		if handler != nil {
			handler.RegisterRoutes(mux)
		}
	}

	return requestLogger(deps.Logger, withCORS(withRequestMeta(mux)))
}

func withRequestMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.RemoteAddr
		}
		if idx := strings.LastIndex(ip, ":"); idx > 0 && !strings.Contains(ip[idx:], "]") {
			ip = ip[:idx]
		}
		ctx := SetRequestMeta(r.Context(), ip, r.Header.Get("User-Agent"))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
