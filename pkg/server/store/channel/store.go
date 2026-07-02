package channel

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/store/empty"
	"github.com/rancher/apiserver/pkg/types"
	"github.com/rancher/channelserver/pkg/config"
	"github.com/rancher/channelserver/pkg/scarf"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
)

type Channel struct {
	empty.Store
	config *config.Config
	scarf  *scarf.Service
}

func New(config *config.Config, svc *scarf.Service) *Channel {
	return &Channel{
		config: config,
		scarf:  svc,
	}
}

func (c *Channel) List(req *types.APIRequest, _ *types.APISchema) (types.APIObjectList, error) {
	req.Type = "channels"
	resp := types.APIObjectList{}
	for _, channel := range c.config.ChannelsConfig().Channels {
		resp.Objects = append(resp.Objects, types.APIObject{
			Type:   "channel",
			ID:     channel.Name,
			Object: channel,
		})
	}
	return resp, nil
}

func (c *Channel) ByID(apiOp *types.APIRequest, schema *types.APISchema, id string) (types.APIObject, error) {
	redirect, version, err := c.config.Redirect(id)
	if err != nil {
		return types.APIObject{}, nil
	}
	if redirect != "" {
		c.scarf.Send(id, version, apiOp.Request)
		http.Redirect(apiOp.Response, apiOp.Request, redirect, http.StatusFound)
		return types.APIObject{}, validation.ErrComplete
	}
	return c.Store.ByID(apiOp, schema, id)
}
