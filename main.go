package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/metrics"
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
	MetricsListenAddress string
	AppName              string
	GithubToken          string

	ScarfEndpoint          string
	ScarfEventInterval     time.Duration
	ScarfClusterLRUMaxSize int
	ScarfTrustedIPs        cli.StringSlice

	GithubApp  config.GithubApp
	URLs       cli.StringSlice
	SubKeys    cli.StringSlice
	PathPrefix cli.StringSlice
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
			Name:        "metrics-listen-address",
			EnvVars:     []string{"METRICS_LISTEN_ADDRESS"},
			Value:       "0.0.0.0:8081",
			Destination: &MetricsListenAddress,
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
			Usage:       "GitHub Auth Token; overrides GitHub App flags if set",
			EnvVars:     []string{"GITHUB_TOKEN"},
			Destination: &GithubToken,
		},
		&cli.Int64Flag{
			Name:        "github-app-id",
			Usage:       "Github App ID",
			EnvVars:     []string{"GITHUB_APP_ID"},
			Destination: &GithubApp.ID,
		},
		&cli.StringFlag{
			Name:        "github-app-private-key",
			Usage:       "Github App RSA Private key; accepts path to file or literal PEM encoded PKCS1 or PKCS8 key",
			EnvVars:     []string{"GITHUB_APP_PRIVATE_KEY"},
			Destination: &GithubApp.PrivateKey,
		},
		&cli.Int64Flag{
			Name:        "github-app-installation-id",
			Usage:       "Github App Installation ID",
			EnvVars:     []string{"GITHUB_APP_INSTALLATION_ID"},
			Destination: &GithubApp.InstallationID,
		},
		&cli.StringFlag{
			Name:        "scarf-endpoint",
			Usage:       "Scarf gateway endpoint template with a {channel} placeholder; empty disables reporting",
			EnvVars:     []string{"SCARF_ENDPOINT"},
			Destination: &ScarfEndpoint,
		},
		&cli.StringSliceFlag{
			Name:        "scarf-trusted-ips",
			Usage:       "IPs or CIDRs to trust to set X-Forwarded-* headers for client addresses reported in Scarf events",
			EnvVars:     []string{"SCARF_TRUSTED_IPS"},
			Destination: &ScarfTrustedIPs,
		},
		&cli.DurationFlag{
			Name:        "scarf-event-interval",
			Usage:       "Interval at which to emit aggregated Scarf events",
			EnvVars:     []string{"SCARF_EVENT_INTERVAL"},
			Value:       24 * time.Hour,
			Destination: &ScarfEventInterval,
		},
		&cli.IntFlag{
			Name:        "scarf-cluster-lru-max-size",
			Usage:       "Maximum size of cluster-id LRU for Scarf event aggregation",
			EnvVars:     []string{"SCARF_CLUSTER_LRU_MAX_SIZE"},
			Value:       scarf.DefaultLRUMaxSize,
			Destination: &ScarfClusterLRUMaxSize,
		},
		&cli.BoolFlag{
			Name:        "debug",
			EnvVars:     []string{"DEBUG"},
			Destination: &Debug,
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
		auth    config.GithubAuth
		rec     config.Recorder
	)

	logrus.SetOutput(os.Stderr)
	ctx, cancel := context.WithCancel(signals.SetupSignalContext())
	defer cancel()
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

	if ScarfEndpoint != "" {
		userAgent := c.App.Name + "/" + c.App.Version
		scarf, err := scarf.New(ctx, ScarfEndpoint, userAgent, ScarfTrustedIPs.Value(), ScarfEventInterval, ScarfClusterLRUMaxSize)
		if err != nil {
			return err
		}
		rec = scarf
	}

	if GithubToken != "" {
		logrus.Info("Using GitHub auth from static token")
		auth = config.GithubToken(GithubToken)
	} else if GithubApp.ID != 0 && GithubApp.InstallationID != 0 && GithubApp.PrivateKey != "" {
		logrus.Infof("Using GitHub auth from AppID %d", GithubApp.ID)
		auth = GithubApp
	} else {
		logrus.Warnf("Using GitHub anonymous auth - may fail to list all releases")
	}

	if len(SubKeys.Value()) != len(PathPrefix.Value()) {
		return fmt.Errorf("keys-prefix lengths are not equal %s %s %s", PathPrefix.Value(), SubKeys.Value(), ListenAddress)
	}

	for _, url := range URLs.Value() {
		sources = append(sources, config.StringSource(url))
	}

	for index, subkey := range SubKeys.Value() {
		prefix := PathPrefix.Value()[index]
		config, err := config.New(ctx,
			config.WithSubkey(subkey),
			config.WithWaiter(waiter),
			config.WithChannelServerVersion(ChannelServerVersion),
			config.WithAppName(AppName),
			config.WithAuth(auth),
			config.WithSources(sources),
			config.WithFatalLoad(RefreshFatal),
			config.WithRecorder(rec),
		)
		if err != nil {
			return fmt.Errorf("failed to create config for %q: %w", prefix, err)
		}
		configs[prefix] = config
		logrus.Infof("Serving channels from %v with subkey %q at /%s", sources, subkey, prefix)
	}

	wg := &sync.WaitGroup{}
	wg.Go(func() {
		if lerr := server.ListenAndServe(ctx, ListenAddress, configs); lerr != nil {
			err = errors.Join(err, lerr)
			cancel()
		}
	})

	if MetricsListenAddress != "" {
		wg.Go(func() {
			if lerr := metrics.ListenAndServe(ctx, MetricsListenAddress, configs); lerr != nil {
				err = errors.Join(err, lerr)
				cancel()
			}
		})
	}

	wg.Wait()
	return err
}
