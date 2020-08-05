package config

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-github/v29/github"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/wrangler/pkg/ticker"
	"github.com/sirupsen/logrus"
)

type Config struct {
	sync.Mutex

	url            string
	redirect       *url.URL
	gh             *github.Client
	channelsConfig *model.ChannelsConfig
	releasesConfig *model.ReleasesConfig
}

func NewConfig(ctx context.Context, subKey string, refresh time.Duration, channelServerVersion string, urls []string) (*Config, error) {

	c := &Config{}
	if _, err := c.loadConfig(ctx, subKey, channelServerVersion, urls...); err != nil {
		return nil, err
	}

	go func() {
		for range ticker.Context(ctx, refresh) {
			if index, err := c.loadConfig(ctx, subKey, channelServerVersion, urls...); err != nil {
				logrus.Errorf("failed to reload configuration from %s: %v", urls, err)
			} else {
				urls = urls[:index+1]
				logrus.Infof("Loaded configuration from %s in %v", urls[index], urls)
			}
		}
	}()

	return c, nil
}

func (c *Config) loadConfig(ctx context.Context, subKey string, channelServerVersion string, urls ...string) (int, error) {
	content, index, err := getURLs(ctx, urls...)
	if err != nil {
		return index, err
	}

	config, err := GetChannelsConfig(ctx, content, subKey)
	if err != nil {
		return index, err
	}

	releases, err := GetReleasesConfig(content, channelServerVersion, subKey)
	if err != nil {
		return index, err
	}

	return index, c.setConfig(ctx, channelServerVersion, config, releases)
}

func (c *Config) ghClient(config *model.ChannelsConfig) (*github.Client, error) {
	if config.GitHub == nil {
		return nil, nil
	}

	if c.gh == nil || c.url != config.GitHub.APIURL {
		if config.GitHub.APIURL == "" {
			return github.NewClient(nil), nil
		}
		return github.NewEnterpriseClient(config.GitHub.APIURL, config.GitHub.APIURL, nil)
	}
	return c.gh, nil
}

func (c *Config) setConfig(ctx context.Context, channelServerVersion string, config *model.ChannelsConfig, releases *model.ReleasesConfig) error {
	gh, err := c.ghClient(config)
	if err != nil {
		return err
	}

	redirect, err := url.Parse(config.RedirectBase)
	if err != nil {
		return err
	}

	var ghReleases []string
	if gh != nil {
		ghReleases, err = GetGHReleases(ctx, gh, config.GitHub.Owner, config.GitHub.Repo)
		if err != nil {
			return err
		}
	}

	if err := resolveChannels(ghReleases, config); err != nil {
		return err
	}

	c.Lock()
	defer c.Unlock()
	c.gh = gh
	c.channelsConfig = config
	c.redirect = redirect
	c.releasesConfig = releases
	if config.GitHub != nil {
		c.url = config.GitHub.APIURL
	}

	return nil
}

func resolveChannels(releases []string, config *model.ChannelsConfig) error {
	for i, channel := range config.Channels {
		if channel.Latest != "" {
			continue
		}
		if channel.LatestRegexp == "" {
			continue
		}

		release, err := Latest(releases, channel.LatestRegexp, channel.ExcludeRegexp)
		if err != nil {
			return err
		}
		config.Channels[i].Latest = release
	}

	return nil
}

func (c *Config) ChannelsConfig() *model.ChannelsConfig {
	c.Lock()
	defer c.Unlock()
	return c.channelsConfig
}

func (c *Config) ReleasesConfig() *model.ReleasesConfig {
	c.Lock()
	defer c.Unlock()
	return c.releasesConfig
}

func (c *Config) Redirect(id string) (string, error) {
	for _, channel := range c.channelsConfig.Channels {
		if channel.Name == id && channel.Latest != "" {
			return c.redirect.ResolveReference(&url.URL{
				Path: channel.Latest,
			}).String(), nil
		}
	}

	return "", nil
}
