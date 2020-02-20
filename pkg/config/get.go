package config

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/google/go-github/v29/github"
	"github.com/hashicorp/go-getter"
	"github.com/rancher/channelserver/pkg/model"
	"sigs.k8s.io/yaml"
)

func get(ctx context.Context, url string) ([]byte, error) {
	content, err := ioutil.ReadFile(url)
	if err == nil {
		return content, nil
	}

	tmp, err := ioutil.TempFile("", "channel-config*.yaml")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	err = getter.GetFile(tmp.Name(), url, getter.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(tmp)
}

func GetConfig(ctx context.Context, configURL string) (*model.ChannelsConfig, error) {
	content, err := get(ctx, configURL)
	if err != nil {
		return nil, err
	}

	config := &model.ChannelsConfig{}
	return config, yaml.Unmarshal(content, config)
}

func GetReleases(ctx context.Context, client *github.Client, owner, repo string) ([]string, error) {
	var (
		opt         = &github.ListOptions{}
		allReleases []string
	)

	for {
		releases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
		if err != nil {
			return nil, err
		}
		for _, release := range releases {
			if release.TagName != nil {
				allReleases = append(allReleases, *release.TagName)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allReleases, nil
}
