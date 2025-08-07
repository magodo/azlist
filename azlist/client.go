package azlist

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	sdkARMResources "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v2"
	"github.com/magodo/azlist/arg"
	"github.com/magodo/azlist/armresources"
)

type Client struct {
	resource      *sdkARMResources.Client
	resourceGroup *sdkARMResources.ResourceGroupsClient
	listresource  *armresources.Client
	resourceGraph *arg.Client
}

func NewClient(subscriptionId string, cred azcore.TokenCredential, clientOpt arm.ClientOptions) (*Client, error) {
	rgClient, err := sdkARMResources.NewResourceGroupsClient(subscriptionId, cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	resClient, err := sdkARMResources.NewClient(subscriptionId, cred, &clientOpt)
	if err != nil {
		return nil, err
	}

	listClient, err := armresources.NewClient(subscriptionId, cred, &clientOpt)
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
		listresource:  listClient,
		resourceGraph: argClient,
	}, nil
}
