// Package scarf reports channel-resolution events to a Scarf gateway.
package scarf

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	channelPlaceholder = "{channel}"
	requestTimeout     = 5 * time.Second
	userAgent          = "channelserver"
)

// Event holds the data reported for a single channel resolution.
type Event struct {
	Channel         string
	ResolvedVersion string
	LatestVersion   string // X-SUC-Latest-Version, set only by system-upgrade-controller
	ClusterID       string // X-SUC-Cluster-ID, set only by system-upgrade-controller
	ClientIP        string
}

// Service sends events to a Scarf gateway endpoint.
type Service struct {
	client   *http.Client
	endpoint string
	limiter  *limiter
}

// New returns a Service reporting to endpoint, rate-limiting per cluster
// within window (0 disables), tracking up to size distinct clusters.
func New(ctx context.Context, endpoint string, window time.Duration, size int) *Service {
	s := &Service{
		client:   &http.Client{Timeout: requestTimeout},
		endpoint: endpoint,
	}
	if endpoint != "" {
		s.limiter = newLimiter(window, size)
		go s.limiter.reportLoop(ctx)
	}
	return s
}

// Send reports a resolved channel in a background goroutine; no-op when
// disabled. Reads headers/IP only past the rate-limit gate; never blocks.
func (s *Service) Send(channel, resolvedVersion string, req *http.Request) {
	if s.endpoint == "" {
		return
	}
	clusterID := req.Header.Get("X-SUC-Cluster-ID")
	if !s.limiter.allow(clusterID) {
		logrus.Debugf("rate-limited Scarf event for cluster %q channel %q", clusterID, channel)
		return
	}
	event := Event{
		Channel:         channel,
		ResolvedVersion: resolvedVersion,
		LatestVersion:   req.Header.Get("X-SUC-Latest-Version"),
		ClusterID:       clusterID,
		ClientIP:        clientIP(req),
	}
	go func() {
		if err := s.send(event); err != nil {
			logrus.Errorf("failed to send Scarf event for channel %q: %v", channel, err)
		} else {
			logrus.Debugf("sent Scarf event for channel %q", channel)
		}
	}()
}

func (s *Service) send(event Event) error {
	endpoint := strings.ReplaceAll(s.endpoint, channelPlaceholder, url.PathEscape(event.Channel))
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	q := u.Query()
	if event.ResolvedVersion != "" {
		q.Set("version_resolved", event.ResolvedVersion)
	}
	if event.LatestVersion != "" {
		q.Set("version_latest", event.LatestVersion)
	}
	if event.ClusterID != "" {
		q.Set("cluster_id", event.ClusterID)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if event.ClientIP != "" {
		req.Header.Set("X-Scarf-IP", event.ClientIP)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// clientIP returns the real client for the X-Scarf-IP header: first
// X-Forwarded-For hop (we run behind a load balancer), else RemoteAddr host.
func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
