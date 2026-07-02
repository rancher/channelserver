package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/scarf"
	"github.com/rancher/channelserver/pkg/server"
	"github.com/rancher/channelserver/pkg/wait"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var (
	Version   = "v0.0.0-dev"
	GitCommit = "HEAD"

	Debug                bool
	RefreshFatal         bool
	RefreshInterval      string
	RefreshSchedule      string
	ChannelServerVersion string
	ListenAddress        string
	AppName              string
	GithubToken          string
	URLs                 cli.StringSlice
	SubKeys              cli.StringSlice
	PathPrefix           cli.StringSlice
	ScarfEndpoint        string
	ScarfRateLimitWindow time.Duration
	ScarfRateLimitSize   int
)

func main() {
	app := cli.NewApp()
	app.Name = "channelserver"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Flags = []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "url",
			EnvVars:     []string{"URL"},
			Value:       cli.NewStringSlice("channels.yaml"),
			Destination: &URLs,
		},
		&cli.StringSliceFlag{
			Name:        "config-key",
			EnvVars:     []string{"SUBKEY"},
			Value:       cli.NewStringSlice(""),
			Destination: &SubKeys,
		},
		&cli.StringFlag{
			Name:        "refresh-interval",
			Usage:       "Time interval between attempted refreshes of the config URL",
			EnvVars:     []string{"REFRESH_INTERVAL"},
			Value:       "15m",
			Destination: &RefreshInterval,
		},
		&cli.StringFlag{
			Name:        "refresh-schedule",
			Usage:       "Cron expression for attempted refreshes of the config URL; overrides refresh-interval if set",
			EnvVars:     []string{"REFRESH_SCHEDULE"},
			Destination: &RefreshSchedule,
		},
		&cli.BoolFlag{
			Name:        "refresh-fatal",
			Usage:       "Exit with a fatal error if config URL refresh fails",
			EnvVars:     []string{"REFRESH_FATAL"},
			Destination: &RefreshFatal,
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
			Usage:       "Name of the app for which to retrieve the app default versions",
			EnvVars:     []string{"APP_NAME"},
			Destination: &AppName,
		},
		&cli.StringSliceFlag{
			Name:        "path-prefix",
			EnvVars:     []string{"PATH_PREFIX"},
			Value:       cli.NewStringSlice("v1-release"),
			Destination: &PathPrefix,
		},
		&cli.StringFlag{
			Name:        "github-token",
			EnvVars:     []string{"GITHUB_TOKEN"},
			Destination: &GithubToken,
		},
		&cli.BoolFlag{
			Name:        "debug",
			EnvVars:     []string{"DEBUG"},
			Destination: &Debug,
		},
		&cli.StringFlag{
			Name:        "scarf-endpoint",
			Usage:       "Scarf gateway endpoint template with a {channel} placeholder; empty disables reporting",
			EnvVars:     []string{"SCARF_ENDPOINT"},
			Destination: &ScarfEndpoint,
		},
		&cli.DurationFlag{
			Name:        "scarf-rate-limit-window",
			Usage:       "per-cluster Scarf event rate-limit window (e.g. 24h); 0 disables",
			EnvVars:     []string{"SCARF_RATE_LIMIT_WINDOW"},
			Value:       24 * time.Hour,
			Destination: &ScarfRateLimitWindow,
		},
		&cli.IntFlag{
			Name:        "scarf-rate-limit-size",
			Usage:       "max distinct cluster IDs tracked for rate limiting",
			EnvVars:     []string{"SCARF_RATE_LIMIT_SIZE"},
			Value:       scarf.DefaultCacheSize,
			Destination: &ScarfRateLimitSize,
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatalf("%s error: %v", app.Name, err)
	}
}

func run(c *cli.Context) error {
	var (
		configs = map[string]*config.Config{}
		sources []config.Source
		waiter  wait.Wait
		err     error
	)

	logrus.SetOutput(os.Stderr)
	ctx := signals.SetupSignalContext()
	if Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	if RefreshSchedule != "" {
		waiter, err = wait.NewSchedule(RefreshSchedule)
	} else {
		waiter, err = wait.NewInterval(RefreshInterval)
	}
	if err != nil {
		return err
	}

	if len(SubKeys.Value()) != len(PathPrefix.Value()) {
		return errors.Errorf("keys-prefix lengths are not equal %s %s %s", PathPrefix.Value(), SubKeys.Value(), ListenAddress)
	}

	for _, url := range URLs.Value() {
		sources = append(sources, config.StringSource(url))
	}
	for index, subkey := range SubKeys.Value() {
		prefix := PathPrefix.Value()[index]
		config := config.NewConfig(ctx, subkey, waiter, ChannelServerVersion, AppName, GithubToken, sources, RefreshFatal)
		configs[prefix] = config
		logrus.Infof("Serving channels from %v with subkey %q at /%s", sources, subkey, prefix)
	}

	if ScarfRateLimitWindow < 0 {
		return errors.Errorf("scarf-rate-limit-window must be >= 0, got %v", ScarfRateLimitWindow)
	}
	svc := scarf.New(ctx, ScarfEndpoint, ScarfRateLimitWindow, ScarfRateLimitSize)
	return server.ListenAndServe(ctx, ListenAddress, configs, svc)
}
