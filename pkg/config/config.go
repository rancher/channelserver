package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-github/v67/github"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/sirupsen/logrus"
)

type Config struct {
	sync.Mutex
	refreshMu sync.Mutex

	subKey               string
	channelServerVersion string
	appName              string
	urls                 []Source

	url               string
	ghToken           string
	redirect          *url.URL
	gh                *github.Client
	channelsConfig    *model.ChannelsConfig
	releasesConfig    *model.ReleasesConfig
	appDefaultsConfig *model.AppDefaultsConfig
}

type Wait interface {
	Wait(ctx context.Context) bool
}

type Source interface {
	URL() string
}

type DurationWait struct {
	Duration time.Duration
}

func (d *DurationWait) Wait(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d.Duration):
		return true
	}
}

type StringSource string

func (s StringSource) URL() string {
	return string(s)
}

func NewConfig(ctx context.Context, subKey string, wait Wait, channelServerVersion string, appName string, ghToken string, urls []Source) *Config {
	c := &Config{
		subKey:               subKey,
		channelServerVersion: channelServerVersion,
		appName:              appName,
		urls:                 urls,

		ghToken:           ghToken,
		channelsConfig:    &model.ChannelsConfig{},
		releasesConfig:    &model.ReleasesConfig{},
		appDefaultsConfig: &model.AppDefaultsConfig{},
	}

	logrus.Infof("Loading configuration from %v", urls)
	if err := c.LoadConfig(ctx); err != nil {
		logrus.Fatalf("Failed to load initial config for %s: %v", subKey, err)
	}

	logrus.Infof("Loaded initial configuration for %s", subKey)

	if wait != nil {
		go func() {
			for wait.Wait(ctx) {
				if err := c.LoadConfig(ctx); err != nil {
					logrus.Errorf("Failed to reload configuration for %s: %v", subKey, err)
				} else {
					logrus.Infof("Reloaded configuration for %s", subKey)
				}
			}
		}()
	}

	return c
}

// Reload the configuration from the source urls. Concurrent loads will
// not block and immediately return an error.
func (c *Config) LoadConfig(ctx context.Context) error {
	locked := c.refreshMu.TryLock()
	if !locked {
		return errors.New("configuration is already being loaded")
	}
	defer c.refreshMu.Unlock()

	content, index, err := getURLs(ctx, c.urls...)
	if err != nil {
		return fmt.Errorf("failed to get content from url %s: %w", c.urls[index].URL(), err)
	}

	config, err := GetChannelsConfig(ctx, content, c.subKey)
	if err != nil {
		return fmt.Errorf("failed to get channel config: %w", err)
	}

	releases, err := GetReleasesConfig(content, c.channelServerVersion, c.subKey)
	if err != nil {
		return fmt.Errorf("failed to get release config: %w", err)
	}

	appDefaultsConfig, err := GetAppDefaultsConfig(content, c.subKey, c.appName)
	if err != nil {
		return fmt.Errorf("failed to get app default config: %w", err)
	}

	err = c.setConfig(ctx, c.channelServerVersion, config, releases, appDefaultsConfig)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}

	c.urls = c.urls[:index+1]

	return nil
}

func (c *Config) ghClient(config *model.ChannelsConfig) (*github.Client, error) {
	if config.GitHub == nil {
		return nil, nil
	}

	if c.gh == nil || c.url != config.GitHub.APIURL {
		client := github.NewClient(nil)
		if c.ghToken != "" {
			client = client.WithAuthToken(c.ghToken)
		}
		if config.GitHub.APIURL != "" {
			return client.WithEnterpriseURLs(config.GitHub.APIURL, config.GitHub.APIURL)
		}
		return client, nil
	}
	return c.gh, nil
}

func (c *Config) setConfig(ctx context.Context, channelServerVersion string, config *model.ChannelsConfig, releases *model.ReleasesConfig, appDefaultsConfig *model.AppDefaultsConfig) error {
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
	c.appDefaultsConfig = appDefaultsConfig
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

func (c *Config) AppDefaultsConfig() *model.AppDefaultsConfig {
	c.Lock()
	defer c.Unlock()
	return c.appDefaultsConfig
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
