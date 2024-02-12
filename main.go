package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/server"
	"github.com/rancher/wrangler/v2/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	Version              = "v0.0.0-dev"
	GitCommit            = "HEAD"
	URLs                 = cli.NewStringSlice("channels.yaml")
	RefreshInterval      string
	ListenAddress        string
	SubKeys              cli.StringSlice
	ChannelServerVersion string
	PathPrefix           cli.StringSlice
	AppName              string
)

func main() {
	app := cli.NewApp()
	app.Name = "Channel Server"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Flags = []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "url",
			EnvVars: []string{"URL"},
			Value:   URLs,
		},
		&cli.StringSliceFlag{
			Name:        "config-key",
			EnvVars:     []string{"SUBKEY"},
			Destination: &SubKeys,
		},
		&cli.StringFlag{
			Name:        "refresh-interval",
			EnvVars:     []string{"REFRESH_INTERVAL"},
			Value:       "15m",
			Destination: &RefreshInterval,
		},
		&cli.StringFlag{
			Name:        "listen-address",
			EnvVars:     []string{"LISTEN_ADDRESS"},
			Value:       "0.0.0.0:8080",
			Destination: &ListenAddress,
		},
		&cli.StringFlag{
			Name:        "channel-server-version",
			EnvVars:     []string{"CHANNEL_SERVER_VERSION"},
			Destination: &ChannelServerVersion,
		},
		&cli.StringFlag{
			Name:        "app-name",
			Usage:       "the app for which to retrieve the app default versions",
			EnvVars:     []string{"APP_NAME"},
			Destination: &AppName,
		},
		&cli.StringSliceFlag{
			Name:        "path-prefix",
			EnvVars:     []string{"PATH_PREFIX"},
			Value:       cli.NewStringSlice("v1-release"),
			Destination: &PathPrefix,
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	logrus.SetOutput(os.Stderr)
	ctx := signals.SetupSignalContext()

	intval, err := time.ParseDuration(RefreshInterval)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s", RefreshInterval)
	}
	if len(SubKeys.Value()) != len(PathPrefix.Value()) {
		return errors.Errorf("keys-prefix lengths are not equal %s %s %s ", PathPrefix.Value(), SubKeys.Value(), ListenAddress)
	}

	var (
		configs = map[string]*config.Config{}
		sources []config.Source
	)

	for _, url := range URLs.Value() {
		sources = append(sources, config.StringSource(url))
	}
	for index, subkey := range SubKeys.Value() {
		config := config.NewConfig(ctx, subkey, &config.DurationWait{Duration: intval}, ChannelServerVersion, AppName, sources)
		configs[PathPrefix.Value()[index]] = config
	}
	return server.ListenAndServe(ctx, ListenAddress, configs)
}
