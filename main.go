package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/server"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	Version         = "v0.0.0-dev"
	GitCommit       = "HEAD"
	URL             string
	RefreshInterval string
	ListenAddress   string
	SubKey          string
)

func main() {
	app := cli.NewApp()
	app.Name = "Chanel Server"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "url",
			EnvVar:      "URL",
			Value:       "channels.yaml",
			Destination: &URL,
		},
		cli.StringFlag{
			Name:        "config-key",
			EnvVar:      "SUBKEY",
			Destination: &SubKey,
		},
		cli.StringFlag{
			Name:        "refresh-interval",
			EnvVar:      "REFRESH_INTERVAL",
			Value:       "15m",
			Destination: &RefreshInterval,
		},
		cli.StringFlag{
			Name:        "listen-address",
			EnvVar:      "LISTEN_ADDRESS",
			Value:       "0.0.0.0:8080",
			Destination: &ListenAddress,
		},
	}
	app.Action = run

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}

func run(c *cli.Context) error {
	flag.Parse()

	logrus.SetOutput(os.Stderr)
	ctx := signals.SetupSignalHandler(context.Background())

	intval, err := time.ParseDuration(RefreshInterval)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %s", RefreshInterval)
	}

	config, err := config.NewConfig(ctx, URL, SubKey, intval)
	if err != nil {
		return err
	}

	return server.ListenAndServe(ctx, ListenAddress, config)
}
