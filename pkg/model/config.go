package model

type ChannelsConfig struct {
	Channels     []Channel `json:"channels,omitempty"`
	GitHub       *GitHub   `json:"github,omitempty"`
	RedirectBase string    `json:"redirectBase,omitempty"`
}

type Channel struct {
	Name          string `json:"name,omitempty"`
	Latest        string `json:"latest,omitempty"`
	LatestRegexp  string `json:"latestRegexp,omitempty"`
	ExcludeRegexp string `json:"excludeRegexp,omitempty"`
}

type GitHub struct {
	APIURL string `json:"api,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
}
