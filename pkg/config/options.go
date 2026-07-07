package config

import "github.com/rancher/channelserver/pkg/wait"

type OptionsFunc func(*Config) error

func WithChannelServerVersion(channelServerVersion string) OptionsFunc {
	return func(c *Config) error {
		c.channelServerVersion = channelServerVersion
		return nil
	}
}

func WithSubkey(subKey string) OptionsFunc {
	return func(c *Config) error {
		c.subKey = subKey
		return nil
	}
}

func WithAppName(appName string) OptionsFunc {
	return func(c *Config) error {
		c.appName = appName
		return nil
	}
}

func WithSources(sources []Source) OptionsFunc {
	return func(c *Config) error {
		c.urls = sources
		return nil
	}
}

func WithAuth(ghAuth GithubAuth) OptionsFunc {
	return func(c *Config) error {
		c.ghAuth = ghAuth
		return nil
	}
}

func WithWaiter(waiter wait.Wait) OptionsFunc {
	return func(c *Config) error {
		c.waiter = waiter
		return nil
	}
}

func WithFatalLoad(fatal bool) OptionsFunc {
	return func(c *Config) error {
		c.fatal = fatal
		return nil
	}
}

func WithRecorder(recorder Recorder) OptionsFunc {
	return func(c *Config) error {
		c.recorder = recorder
		return nil
	}
}

func WithUIBaseURL(baseURL string) OptionsFunc {
	return func(c *Config) error {
		c.uiBase = baseURL
		return nil
	}
}
