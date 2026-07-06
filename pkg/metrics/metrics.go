package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rancher/channelserver/pkg/config"
)

func ListenAndServe(ctx context.Context, address string, configs map[string]*config.Config) error {
	router := http.NewServeMux()
	server := http.Server{
		Addr:    address,
		Handler: router,
	}

	router.Handle("/metrics", promhttp.Handler())
	router.Handle("/livez", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for index, config := range configs {
			if !config.IsValid() {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(fmt.Appendf(nil, "configuration for %q is stale\r\n", index))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok\r\n"))
	}))

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
