package model

import "github.com/rancher/wrangler/pkg/schemas"

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
	Version                 string                   `json:"version,omitempty"`
	ChannelServerMinVersion string                   `json:"minChannelServerVersion,omitempty"`
	ChannelServerMaxVersion string                   `json:"maxChannelServerVersion,omitempty"`
	ServerArgs              map[string]schemas.Field `json:"serverArgs,omitempty"`
	AgentArgs               map[string]schemas.Field `json:"agentArgs,omitempty"`
	CNIValues               map[string]string        `json:"cniValues,omitempty"`
}

type GitHub struct {
	APIURL string `json:"api,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
}
