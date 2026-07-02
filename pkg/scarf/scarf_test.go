package scarf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_UnitSendSUC(t *testing.T) {
	got := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got <- r
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	err := New(ctx, srv.URL+"/rke2-channelserver/{channel}", 0, 0).send(Event{
		Channel:         "stable",
		ResolvedVersion: "v1.31.5+rke2r1",
		LatestVersion:   "v1.31.4+rke2r1",
		ClusterID:       "abc-123",
		ClientIP:        "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	req := <-got
	if req.URL.Path != "/rke2-channelserver/stable" {
		t.Errorf("path = %q, want /rke2-channelserver/stable", req.URL.Path)
	}
	q := req.URL.Query()
	for key, want := range map[string]string{
		"version_resolved": "v1.31.5+rke2r1",
		"version_latest":   "v1.31.4+rke2r1",
		"cluster_id":       "abc-123",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("query %s = %q, want %q", key, got, want)
		}
	}
	if got := req.Header.Get("X-Scarf-IP"); got != "203.0.113.7" {
		t.Errorf("X-Scarf-IP = %q, want 203.0.113.7", got)
	}
}

func Test_UnitSendInstall(t *testing.T) {
	got := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got <- r
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	err := New(ctx, srv.URL+"/rke2-channelserver/{channel}", 0, 0).send(Event{
		Channel:         "stable",
		ResolvedVersion: "v1.31.5+rke2r1",
		ClientIP:        "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	req := <-got
	q := req.URL.Query()
	if q.Has("version_latest") {
		t.Errorf("version_latest should be absent for install.sh requests")
	}
	if q.Has("cluster_id") {
		t.Errorf("cluster_id should be absent for install.sh requests")
	}
}
