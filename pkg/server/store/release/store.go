package release

import (
	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
)

type Store struct {
	empty.Store
	config *config.Config
}

func New(config *config.Config) *Store {
	return &Store{
		config: config,
	}
}

func (c *Store) List(req *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	req.Type = "releases"
	resp := types.APIObjectList{}
	for _, release := range c.config.ReleasesConfig().Releases {
		resp.Objects = append(resp.Objects, types.APIObject{
			Type:   "release",
			ID:     release.Version,
			Object: release,
		})
	}
	return resp, nil
}
