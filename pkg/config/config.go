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

	url      string
	redirect *url.URL
	gh       *github.Client
	config   *model.ChannelsConfig
}

func NewConfig(ctx context.Context, subKey string, refresh time.Duration, urls ...string) (*Config, error) {
	c := &Config{}
	if _, err := c.loadConfig(ctx, subKey, urls...); err != nil {
		return nil, err
	}

	go func() {
		for range ticker.Context(ctx, refresh) {
			if index, err := c.loadConfig(ctx, subKey, urls...); err != nil {
				logrus.Errorf("failed to reload configuration from %s: %v", urls, err)
			} else {
				urls = urls[:index+1]
				logrus.Infof("Loaded configuration from %s in %v", urls[index], urls)
			}
		}
	}()

	return c, nil
}

func (c *Config) loadConfig(ctx context.Context, subKey string, urls ...string) (int, error) {
	config, index, err := GetConfig(ctx, subKey, urls...)
	if err != nil {
		return index, err
	}

	return index, c.setConfig(ctx, config)
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

func (c *Config) setConfig(ctx context.Context, config *model.ChannelsConfig) error {
	gh, err := c.ghClient(config)
	if err != nil {
		return err
	}

	redirect, err := url.Parse(config.RedirectBase)
	if err != nil {
		return err
	}

	var releases []string
	if gh != nil {
		releases, err = GetReleases(ctx, gh, config.GitHub.Owner, config.GitHub.Repo)
		if err != nil {
			return err
		}
	}

	if err := resolveChannels(releases, config); err != nil {
		return err
	}

	c.Lock()
	defer c.Unlock()
	c.gh = gh
	c.config = config
	c.redirect = redirect
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

func (c *Config) Config() *model.ChannelsConfig {
	c.Lock()
	defer c.Unlock()
	return c.config
}

func (c *Config) Redirect(id string) (string, error) {
	for _, channel := range c.config.Channels {
		if channel.Name == id && channel.Latest != "" {
			return c.redirect.ResolveReference(&url.URL{
				Path: channel.Latest,
			}).String(), nil
		}
	}

	return "", nil
}
