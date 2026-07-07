package scarf

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	scarftest "github.com/rancher/channelserver/tests/scarf"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"

	//revive:disable:dot-imports
	. "github.com/onsi/gomega"
)

func init() {
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
}

//nolint:forbidigo,goconst
func Test_ScarfReport(t *testing.T) {
	type request struct {
		clusterID       string
		clientIP        string
		channel         string
		latestVersion   string
		resolvedVersion string
	}

	tests := []struct {
		name           string
		trustedIPs     []string
		reportInterval time.Duration
		lruMaxSize     int
		requests       []request
		wantReport     int
		wantIP         gomock.Matcher
	}{
		{
			name:           "only valid uids are aggregated",
			reportInterval: time.Minute,
			lruMaxSize:     10,
			requests: []request{
				request{},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "bogus"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556"},
			},
			wantReport: 2,
			wantIP:     gomock.Any(),
		},
		{
			name:           "lru size is respected",
			reportInterval: time.Minute,
			lruMaxSize:     1,
			requests: []request{
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556"},
			},
			wantReport: 4,
			wantIP:     gomock.Any(),
		},
		{
			name:           "lru ttl is respected",
			reportInterval: time.Millisecond,
			lruMaxSize:     10,
			requests: []request{
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555555"},
			},
			wantReport: 4,
			wantIP:     gomock.Any(),
		},
		{
			name:           "headers from untrusted hosts are not used",
			reportInterval: time.Minute,
			lruMaxSize:     10,
			requests: []request{
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556", clientIP: "1.2.3.4"},
			},
			wantReport: 1,
			wantIP:     gomock.Eq("192.0.2.1"),
		},
		{
			name:           "headers from trusted hosts are used",
			reportInterval: time.Minute,
			lruMaxSize:     10,
			trustedIPs:     []string{"192.0.0.0/8"},
			requests: []request{
				request{channel: "latest", resolvedVersion: "v1.2.3+foo1", clusterID: "11111111-2222-3333-4444-555555555556", clientIP: "1.2.3.4"},
			},
			wantReport: 1,
			wantIP:     gomock.Eq("1.2.3.4"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			logrus.SetOutput(t.Output())
			ctrl := gomock.NewController(t)
			mockGateway := scarftest.NewMockGateway(ctrl)
			mockGateway.EXPECT().RecordEvent(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), tt.wantIP).Times(tt.wantReport)

			server := httptest.NewServer(scarftest.HandlerFor(mockGateway))
			recorder, err := New(t.Context(), server.URL+"/channels/{channel}", "test", tt.trustedIPs, tt.reportInterval, tt.lruMaxSize)
			g.Expect(err).ToNot(HaveOccurred())

			for _, request := range tt.requests {
				req := httptest.NewRequest("GET", "/v1-release/channels/"+request.channel, nil)
				req.Header.Add("X-SUC-Cluster-ID", request.clusterID)
				req.Header.Add("X-SUC-Latest-Version", request.latestVersion)
				req.Header.Add("X-Forwarded-For", request.clientIP)
				recorder.Redirect(request.channel, request.resolvedVersion, req)
				// wait for workqueue to handle request
				time.Sleep(time.Millisecond * 5)
			}

			server.Close()
			// discard additional log output; test will fail if writer sees output after the test is done
			logrus.SetOutput(io.Discard)
		})
	}
}
