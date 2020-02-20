package server

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/channelserver/pkg/server/store"
	"github.com/rancher/steve/pkg/schemaserver/server"
	"github.com/rancher/steve/pkg/schemaserver/store/apiroot"
	"github.com/rancher/steve/pkg/schemaserver/types"
)

func ListenAndServe(ctx context.Context, address string, config *config.Config) error {
	server := server.DefaultAPIServer()
	server.Schemas.MustImportAndCustomize(model.Channel{}, func(schema *types.APISchema) {
		schema.Store = store.New(config)
		schema.CollectionMethods = []string{http.MethodGet}
		schema.ResourceMethods = []string{http.MethodGet}
	})
	apiroot.Register(server.Schemas, []string{"v1"}, nil)

	router := mux.NewRouter()
	router.MatcherFunc(setType("apiRoot")).Path("/").Handler(server)
	router.MatcherFunc(setType("apiRoot")).Path("/{name}").Handler(server)
	router.Path("/{prefix:v1}/{type}").Handler(server)
	router.Path("/{prefix:v1}/{type}/{name}").Handler(server)

	next := handlers.LoggingHandler(os.Stdout, router)
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user := req.Header.Get("X-SUC-Cluster-ID")
		if user != "" && req.URL != nil {
			req.URL.User = url.User(user)
		}
		next.ServeHTTP(rw, req)
	})

	return http.ListenAndServe(address, handler)
}

func setType(t string) mux.MatcherFunc {
	return func(request *http.Request, match *mux.RouteMatch) bool {
		if match.Vars == nil {
			match.Vars = map[string]string{}
		}
		match.Vars["type"] = t
		match.Vars["prefix"] = "v1"
		return true
	}
}
