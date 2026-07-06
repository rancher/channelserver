package config

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-github/v88/github"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/channelserver/pkg/wait"
	"github.com/sirupsen/logrus"
)

type Wait wait.Wait // retained in this package for backwards compatibility.

// Recorder is a generic interface for recording channel server client responses.
type Recorder interface {
	// Redirect records successful lookup of the latest version for a channel
	Redirect(channel, version string, request *http.Request)
}

// Config holds settings for an instance of an app whose channels, releases, and defaults
// are exposed via the channel server API.
type Config struct {
	sync.Mutex
	refreshMu sync.Mutex

	valid                bool
	fatal                bool
	subKey               string
	channelServerVersion string
	appName              string
	urls                 []Source
	url                  string
	ghAuth               GithubAuth
	recorder             Recorder
	waiter               wait.Wait
	redirect             *url.URL
	gh                   *github.Client

	channelsConfig    *model.ChannelsConfig
	releasesConfig    *model.ReleasesConfig
	appDefaultsConfig *model.AppDefaultsConfig
}

type Source interface {
	URL() string
}

type StringSource string

func (s StringSource) URL() string {
	return string(s)
}

// New creates a new config from options.
// Use `WithWaiter(w Wait)` to automatically reload config.
func New(ctx context.Context, opts ...OptionsFunc) (*Config, error) {
	c := &Config{
		channelsConfig:    &model.ChannelsConfig{},
		releasesConfig:    &model.ReleasesConfig{},
		appDefaultsConfig: &model.AppDefaultsConfig{},
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// Only load config on startup and poll the waiter, if waiter is not nil.
	// If waiter is nil, the external source handles calling `c.LoadConfig()` directly, as needed.
	if c.waiter != nil {
		tryLoadConfig := func(ctx context.Context) {
			start := time.Now()
			if err := c.LoadConfig(ctx); err != nil {
				if c.fatal {
					logrus.Fatalf("Failed to load configuration for subkey %q: %v", c.subKey, err)
				} else {
					logrus.Errorf("Failed to load configuration for subkey %q: %v", c.subKey, err)
				}
				c.valid = false
			} else {
				logrus.Infof("Loaded configuration for subkey %q in %s", c.subKey, time.Since(start))
				c.valid = true
			}
		}

		logrus.Infof("Loading configuration from %v", c.urls)
		tryLoadConfig(ctx)

		go func() {
			for c.waiter.Wait(ctx) {
				tryLoadConfig(ctx)
			}
		}()
	}

	return c, nil
}

// NewConfig returns a config with values preloaded.
//
// Deprecated: NewConfig exists for historical compatibility and should not be used.
// Use `config.New(ctx context.Context, config.WithWaiter(wait config.Wait))` instead.
func NewConfig(ctx context.Context, subKey string, wait Wait, channelServerVersion string, appName string, ghAuth GithubAuth, urls []Source) *Config {
	c, err := New(ctx,
		WithSubkey(subKey),
		WithWaiter(wait),
		WithChannelServerVersion(channelServerVersion),
		WithAppName(appName),
		WithAuth(ghAuth),
		WithSources(urls),
	)
	if err != nil {
		logrus.Fatalf("Failed to create config for subkey %q: %v", subKey, err)
		return nil
	}
	return c
}

// NewConfigNoLoad returns a config without values preloaded.
//
// Deprecated: NewConfigNoLoad exists for historical compatibility and should not be used.
// Use `config.New(ctx context.Context)` instead.
func NewConfigNoLoad(ctx context.Context, subKey string, channelServerVersion string, appName string, ghAuth GithubAuth, urls []Source) *Config {
	c, err := New(ctx,
		WithSubkey(subKey),
		WithChannelServerVersion(channelServerVersion),
		WithAppName(appName),
		WithAuth(ghAuth),
		WithSources(urls),
	)
	if err != nil {
		logrus.Fatalf("Failed to create config for subkey %q: %v", subKey, err)
		return nil
	}
	return c
}

// IsValid returns true if the most recent config load succeeded.
// If this returns false, config may be empty or stale.
func (c *Config) IsValid() bool {
	return c != nil && c.valid
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
		opts := []github.ClientOptionsFunc{github.WithHTTPClient(httpClient)}
		if c.ghAuth != nil {
			authOpts, err := c.ghAuth.ClientOptions()
			if err != nil {
				return nil, err
			}
			opts = append(opts, authOpts...)
		}
		if config.GitHub.APIURL != "" {
			opts = append(opts, github.WithEnterpriseURLs(config.GitHub.APIURL, config.GitHub.APIURL))
		}
		return github.NewClient(opts...)
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

func (c *Config) Redirect(id string, apiOp *types.APIRequest) (string, error) {
	for _, channel := range c.channelsConfig.Channels {
		if channel.Name == id && channel.Latest != "" {
			if c.recorder != nil && apiOp != nil {
				c.recorder.Redirect(id, channel.Latest, apiOp.Request)
			}
			return c.redirect.ResolveReference(&url.URL{
				Path: channel.Latest,
			}).String(), nil
		}
	}

	return "", nil
}
