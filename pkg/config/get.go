package config

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/blang/semver"
	"github.com/google/go-github/v29/github"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/wrangler/pkg/data/convert"
	"sigs.k8s.io/yaml"
)

func getURLs(ctx context.Context, urls ...Source) ([]byte, int, error) {
	var (
		bytes []byte
		err   error
		index int
	)
	for i, url := range urls {
		index = i
		bytes, err = get(ctx, url)
		if err == nil {
			break
		}
	}

	return bytes, index, err
}

func get(ctx context.Context, url Source) ([]byte, error) {
	content, err := ioutil.ReadFile(url.URL())
	if err == nil {
		return content, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.URL(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return ioutil.ReadAll(resp.Body)
}

func GetChannelsConfig(ctx context.Context, content []byte, subKey string) (*model.ChannelsConfig, error) {
	var (
		data   = map[string]interface{}{}
		config = &model.ChannelsConfig{}
	)

	if subKey == "" {
		return config, yaml.Unmarshal(content, config)
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	data, _ = data[subKey].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("failed to find key %s in config", subKey)
	}
	return config, convert.ToObj(data, config)
}

func GetReleasesConfig(content []byte, channelServerVersion, subKey string) (*model.ReleasesConfig, error) {
	var (
		allReleases       model.ReleasesConfig
		availableReleases model.ReleasesConfig
		data              map[string]interface{}
	)

	if subKey == "" {
		if err := yaml.Unmarshal(content, &allReleases); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
		data, _ = data[subKey].(map[string]interface{})
		if err := convert.ToObj(data, &allReleases); err != nil {
			return nil, err
		}
	}

	if err := yaml.Unmarshal(content, &allReleases); err != nil {
		return nil, err
	}

	// no server version specified, show all releases
	if channelServerVersion == "" {
		return &allReleases, nil
	}

	availableReleases = model.ReleasesConfig{}

	serverVersion, err := semver.ParseTolerant(channelServerVersion)
	if err != nil {
		return nil, err
	}

	for _, release := range allReleases.Releases {
		minServerVer, err := semver.ParseTolerant(release.ChannelServerMinVersion)
		if err != nil {
			continue
		}

		maxServerVer, err := semver.ParseTolerant(release.ChannelServerMaxVersion)
		if err != nil {
			continue
		}

		if serverVersion.GE(minServerVer) && serverVersion.LE(maxServerVer) {
			availableReleases.Releases = append(availableReleases.Releases, release)
		}
	}

	return &availableReleases, nil
}

func GetGHReleases(ctx context.Context, client *github.Client, owner, repo string) ([]string, error) {
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
			if release.GetTagName() != "" && !release.GetPrerelease() {
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
