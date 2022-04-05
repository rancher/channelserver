module github.com/rancher/channelserver

go 1.16

replace k8s.io/client-go => k8s.io/client-go v0.20.0

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/google/go-github/v29 v29.0.3
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.7.3
	github.com/pkg/errors v0.9.1
	github.com/rancher/apiserver v0.0.0-20201023000256-1a0a904f9197
	github.com/rancher/wrangler v0.8.11-0.20220120160420-18c996a8e956
	github.com/sirupsen/logrus v1.6.0
	github.com/urfave/cli/v2 v2.4.0
	sigs.k8s.io/yaml v1.2.0
)
