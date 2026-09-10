package model

import "github.com/rancher/wrangler/v3/pkg/schemas"

type ChannelsConfig struct {
	Channels     []Channel `json:"channels,omitempty"`
	GitHub       *GitHub   `json:"github,omitempty"`
	RedirectBase string    `json:"redirectBase,omitempty"`
}

type ReleasesConfig struct {
	Releases []Release `json:"releases,omitempty"`
}

type Channel struct {
	Name          string `json:"name,omitempty"`
	Latest        string `json:"latest,omitempty"`
	LatestRegexp  string `json:"latestRegexp,omitempty"`
	ExcludeRegexp string `json:"excludeRegexp,omitempty"`
}

type Release struct {
	Version                 string            `json:"version,omitempty"`
	ChannelServerMinVersion string            `json:"minChannelServerVersion,omitempty"`
	ChannelServerMaxVersion string            `json:"maxChannelServerVersion,omitempty"`
	ServerArgs              map[string]Arg    `json:"serverArgs,omitempty"`
	AgentArgs               map[string]Arg    `json:"agentArgs,omitempty"`
	FeatureVersions         map[string]string `json:"featureVersions,omitempty"`
	Charts                  map[string]Chart  `json:"charts,omitempty"`
}

// Arg describes a single K3s/RKE2 server or agent argument as published by KDM.
// The embedded schemas.Field is inlined on the wire, so the serialized form is a
// flat object.
type Arg struct {
	schemas.Field
	// AgentRestart marks a server-only argument whose value affects agent runtime
	// behavior. Such an argument is stripped from worker configuration, so consumers
	// must account for it separately to restart workers when it changes.
	AgentRestart bool `json:"agentRestart,omitempty"`
}

type Chart struct {
	Repo    string `json:"repo,omitempty"`
	Version string `json:"version,omitempty"`
}

type GitHub struct {
	APIURL string `json:"api,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
}

type AppDefaultsConfig struct {
	AppDefaults []AppDefault `json:"appDefaults,omitempty"`
}

type AppDefault struct {
	AppName  string    `json:"appName,omitempty"`
	Defaults []Default `json:"defaults,omitempty"`
}

type Default struct {
	AppVersion     string `json:"appVersion,omitempty"`
	DefaultVersion string `json:"defaultVersion,omitempty"`
}
