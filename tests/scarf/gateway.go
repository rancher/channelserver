package scarf

import (
	"net"
	"net/http"
)

func HandlerFor(g Gateway) http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /channels/{channel}", func(rw http.ResponseWriter, req *http.Request) {
		channel := req.PathValue("channel")
		resolved := req.URL.Query().Get("version_resolved")
		latest := req.URL.Query().Get("version_latest")
		clusterID := req.URL.Query().Get("cluster_id")
		clientIP := req.Header.Get("X-Scarf-IP")
		if clientIP == "" {
			clientIP, _, _ = net.SplitHostPort(req.RemoteAddr)
		}
		g.RecordEvent(channel, resolved, latest, clusterID, clientIP)
		rw.Header().Add("Location", "https://github.com/k3s-io/k3s/releases/tag/"+resolved)
		rw.WriteHeader(http.StatusTemporaryRedirect)
	})
	return router
}
