package scarf

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rancher/channelserver/pkg/config"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/operation"
	"k8s.io/apimachinery/pkg/api/validate"
	"k8s.io/apimachinery/pkg/util/cache"
	apiutils "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	netutils "k8s.io/utils/net"

	_ "k8s.io/component-base/metrics/prometheus/workqueue"
)

const (
	DefaultLRUMaxSize = 64000
)

var (
	defaultWorkers     = 4
	channelPlaceholder = "{channel}"

	httpClient = http.Client{
		Timeout:       5 * time.Second,
		Transport:     &config.LoggingTransport{},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }, // do not follow redirects
	}
)

type event struct {
	clientIP        string
	channel         string
	latestVersion   string
	resolvedVersion string
}

type Recorder struct {
	endpoint     string
	userAgent    string
	cache        *cache.LRUExpireCache
	workqueue    workqueue.TypedInterface[string]
	trustedCIDRs []*net.IPNet
	ttl          time.Duration
}

// New constructs a new Scarf event recorder with the provided configuration
func New(ctx context.Context, endpoint, userAgent string, trustedIPs []string, reportInterval time.Duration, lruMaxSize int) (*Recorder, error) {
	s := &Recorder{
		endpoint:  endpoint,
		userAgent: userAgent,
		cache:     cache.NewLRUExpireCache(lruMaxSize),
		workqueue: workqueue.NewTypedWithConfig[string](workqueue.TypedQueueConfig[string]{Name: "ScarfRecorder"}),
		ttl:       reportInterval,
	}
	for _, trusted := range trustedIPs {
		if _, cidr, _ := netutils.ParseCIDRSloppy(trusted); cidr != nil {
			s.trustedCIDRs = append(s.trustedCIDRs, cidr)
		}
	}
	go s.run(ctx, defaultWorkers)
	return s, nil
}

// Redirect is required to meet the config.Recorder interface. It creates and
// enqueues an new scarf event, only if the cluster-id given by the client is
// valid and not present in the cache.
func (s *Recorder) Redirect(channel, version string, req *http.Request) {
	if clusterID := s.getClusterID(req); clusterID != "" {
		if _, ok := s.cache.Get(clusterID); !ok {
			s.cache.Add(clusterID, &event{
				clientIP:        s.getClientAddr(req),
				channel:         channel,
				latestVersion:   s.getLatestVersion(req),
				resolvedVersion: version,
			}, s.ttl)
			s.workqueue.Add(clusterID)
		}
	}
}

// run starts workqueue goroutines to handle events, and stops the workqueue
// when the context is cancelled. It will block until the context is cancelled.
func (s *Recorder) run(ctx context.Context, workers int) {
	defer s.workqueue.ShutDownWithDrain()
	for _ = range workers {
		go wait.UntilWithContext(ctx, func(ctx context.Context) {
			for s.processNextWorkItem(ctx) {
			}
		}, time.Second)
	}
	<-ctx.Done()
}

// processNextWorkItem retrieves a single item from the workqueue, along with a
// bool indicating if processing should continue.
func (s *Recorder) processNextWorkItem(ctx context.Context) bool {
	key, shutdown := s.workqueue.Get()
	if shutdown {
		return false
	}
	if err := s.processSingleItem(ctx, key); err != nil {
		logrus.Errorf("Scarf workqueue failed to relay event: %v", err)
	}
	return true
}

// processSingleItem calls sendReport to handle the queued event, and calls
// workqueue.Done to remove it from the queue and prevent reprocessing. If
// handling fails, the cache entry is deleted so that the next request with
// this cluster-id can trigger a retry.
func (s *Recorder) processSingleItem(ctx context.Context, key string) error {
	defer s.workqueue.Done(key)
	if err := s.sendReport(ctx, key); err != nil {
		// don't retry this event, but remove cache entry so next request can re-trigger
		s.cache.Remove(key)
		return err
	}
	return nil
}

// sendReport retrieves the event for the queued cluster-id, and converts it to
// a request to the scarf endpoint. If the request is successful, the cache
// entry for this cluster-id is replaced with a nil event. The nil entry will
// prevent additional events from being generated until the ttl expires, but
// will not consume as much memory as a full event.
func (s *Recorder) sendReport(ctx context.Context, clusterID string) error {
	obj, ok := s.cache.Get(clusterID)
	if !ok {
		return nil
	}
	if event, ok := obj.(*event); ok {
		endpoint := strings.ReplaceAll(s.endpoint, channelPlaceholder, url.PathEscape(event.channel))
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}

		q := req.URL.Query()
		q.Set("version_resolved", event.resolvedVersion)
		q.Set("version_latest", event.latestVersion)
		q.Set("cluster_id", clusterID)
		req.URL.RawQuery = q.Encode()
		req.Header.Set("X-Scarf-IP", event.clientIP)
		req.Header.Set("User-Agent", s.userAgent)

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		s.cache.Add(clusterID, nil, s.ttl)
	}
	return nil
}

// getClusterID returns the cluster-id provided by the client. Cluster-ids that
// are not valid Kubernetes UUIDs are not returned.
func (s *Recorder) getClusterID(req *http.Request) string {
	clusterID := req.Header.Get("X-SUC-Cluster-ID")
	if err := validate.UUID(req.Context(), operation.Operation{}, field.NewPath(""), &clusterID, nil); err.ToAggregate() == nil {
		return clusterID
	}
	return ""
}

// getLatestVersion returns the current "latest version" reported by the
// client. Versions that do not appear to be valid version strings are not
// returned.
func (s *Recorder) getLatestVersion(req *http.Request) string {
	latest := req.Header.Get("X-SUC-Latest-Version")
	if v, err := version.Parse(latest); err == nil {
		return "v" + v.String()
	}
	return ""
}

// getClientAddr returns the request's RemoteAddr. If RemoteAddr is in the
// trusted CIDR list and the request has X-Forwarded-For or X-Real-IP headers,
// the first header value is returned instead.
func (s *Recorder) getClientAddr(req *http.Request) string {
	addr, _, _ := net.SplitHostPort(req.RemoteAddr)
	if ip := net.ParseIP(addr); ip != nil {
		for _, cidr := range s.trustedCIDRs {
			if cidr.Contains(ip) {
				if ips := apiutils.SourceIPs(req); len(ips) != 0 {
					return ips[0].String()
				}
				return addr
			}
		}
	}
	return addr
}
