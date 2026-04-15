package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/rancher/apiserver/pkg/server"
	"github.com/rancher/apiserver/pkg/store/apiroot"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/channelserver/pkg/server/store/appdefault"
	"github.com/rancher/channelserver/pkg/server/store/channel"
	"github.com/rancher/channelserver/pkg/server/store/release"
)

func ListenAndServe(ctx context.Context, address string, configs map[string]*config.Config) error {
	h := NewHandler(configs)

	next := LoggingHandler(os.Stdout, h)
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user := req.Header.Get("X-SUC-Cluster-ID")
		if user != "" && req.URL != nil {
			req.URL.User = url.User(user)
		}
		next.ServeHTTP(rw, req)
	})

	return http.ListenAndServe(address, handler)
}

func NewHandler(configs map[string]*config.Config) http.Handler {
	var apiserver *server.Server
	router := http.NewServeMux()
	for prefix, config := range configs {
		apiserver = server.DefaultAPIServer()
		apiserver.Schemas.MustImportAndCustomize(model.Channel{}, func(schema *types.APISchema) {
			schema.Store = channel.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
			schema.ResourceMethods = []string{http.MethodGet}
		})
		apiserver.Schemas.MustImportAndCustomize(model.Release{}, func(schema *types.APISchema) {
			schema.Store = release.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
		})
		apiserver.Schemas.MustImportAndCustomize(model.AppDefault{}, func(schema *types.APISchema) {
			schema.Store = appdefault.New(config)
			schema.CollectionMethods = []string{http.MethodGet}
		})
		prefix = strings.Trim(prefix, "/")
		apiroot.Register(apiserver.Schemas, []string{prefix})
		router.Handle("/"+prefix+"/{type}", setPathValues(apiserver, "", prefix))
		router.Handle("/"+prefix+"/{type}/{name}", setPathValues(apiserver, "", prefix))
	}
	if apiserver != nil {
		router.Handle("/{$}", setPathValues(apiserver, "apiRoot", ""))
		router.Handle("/{name}", setPathValues(apiserver, "apiRoot", ""))
	}
	return router
}

func setPathValues(handler http.Handler, typeName, prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if typeName != "" {
			r.SetPathValue("type", typeName)
		}
		if prefix != "" {
			r.SetPathValue("prefix", prefix)
		}
		handler.ServeHTTP(w, r)
	})
}
