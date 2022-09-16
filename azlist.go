package main

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/armid"
)

func ptr[T any](v T) *T {
	return &v
}

type AzureResource struct {
	Id         armid.ResourceId
	Properties map[string]interface{}
}

func List(ctx context.Context, subscriptionId, query string) (map[string]map[string]interface{}, error) {
	client, err := NewClient(subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	const top int32 = 1000

	queryReq := armresourcegraph.QueryRequest{
		Query: &query,
		Options: &armresourcegraph.QueryRequestOptions{
			AuthorizationScopeFilter: ptr(armresourcegraph.AuthorizationScopeFilterAtScopeAndBelow),
			ResultFormat:             ptr(armresourcegraph.ResultFormatObjectArray),
			Top:                      ptr(top),
		},
		Subscriptions: []*string{&subscriptionId},
	}

	resp, err := client.resourceGraph.Resources(ctx, queryReq, nil)
	if err != nil {
		return nil, fmt.Errorf("executing ARG query %q: %v", query, err)
	}

	var rl []AzureResource

	collectResource := func(resp armresourcegraph.QueryResponse) error {
		for _, resource := range resp.Data.([]interface{}) {
			resource := resource.(map[string]interface{})
			id := resource["id"].(string)
			azureId, err := armid.ParseResourceId(id)
			if err != nil {
				return fmt.Errorf("parsing resource id %s: %v", id, err)
			}
			rl = append(rl, AzureResource{
				Id:         azureId,
				Properties: resource,
			})
		}
		return nil
	}

	if err := collectResource(resp.QueryResponse); err != nil {
		return nil, err
	}

	var total int64
	if resp.TotalRecords != nil {
		total = *resp.TotalRecords
	}

	var count int64
	if resp.Count != nil {
		count = *resp.Count
	}

	var skip int32 = top

	var skipToken string
	if resp.SkipToken != nil {
		skipToken = *resp.SkipToken
	}

	// Should we check for the existance of skipToken instead? But can't find any document states that the last response won't return the skipToken.
	for count < total {
		queryReq.Options.Skip = &skip
		queryReq.Options.SkipToken = &skipToken

		resp, err := client.resourceGraph.Resources(ctx, queryReq, nil)
		if err != nil {
			return nil, fmt.Errorf("running ARG query %q with skipToken %q: %v", query, skipToken, err)
		}

		if err := collectResource(resp.QueryResponse); err != nil {
			return nil, err
		}

		// Update count
		if resp.Count != nil {
			count += *resp.Count
		}

		// Update query controls
		skip += top
		if resp.SkipToken != nil {
			skipToken = *resp.SkipToken
		}
	}

	out := map[string]map[string]interface{}{}
	for _, res := range rl {
		out[res.Id.String()] = res.Properties
	}
	return out, nil
}
