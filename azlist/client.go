package azlist

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	sdkARMResources "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/magodo/azlist/arg"
	"github.com/magodo/azlist/armresources"
)

type Client struct {
	resourceGroup *sdkARMResources.ResourceGroupsClient
	resource      *armresources.Client
	resourceGraph *arg.Client
}

func NewClient(subscriptionId string, cred azcore.TokenCredential, clientOpt arm.ClientOptions) (*Client, error) {
	rgClient, err := sdkARMResources.NewResourceGroupsClient(subscriptionId, cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	resClient, err := armresources.NewClient(subscriptionId, cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	argClient, err := arg.NewClient(cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	return &Client{
		resourceGroup: rgClient,
		resource:      resClient,
		resourceGraph: argClient,
	}, nil
}
