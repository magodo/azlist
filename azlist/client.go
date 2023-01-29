package azlist

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/azlist/armresources"
)

type Client struct {
	resource      *armresources.Client
	resourceGraph *armresourcegraph.Client
}

func NewClient(subscriptionId string, cred azcore.TokenCredential, clientOpt arm.ClientOptions) (*Client, error) {
	resClient, err := armresources.NewClient(subscriptionId, cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	argClient, err := armresourcegraph.NewClient(cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	return &Client{
		resource:      resClient,
		resourceGraph: argClient,
	}, nil
}
