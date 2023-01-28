package azlist

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/azlist/armresources"
	"github.com/magodo/azure-sdk-for-go-helper/authentication"
)

type Client struct {
	resource      *armresources.Client
	resourceGraph *armresourcegraph.Client
}

func NewClient(subscriptionId string, authOpt authentication.Option, clientOpt arm.ClientOptions) (*Client, error) {
	cred, err := authentication.NewAzureCredential(&authOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a credential: %v", err)
	}

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
