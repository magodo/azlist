package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

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

//go:embed armschema.json
var armSchemaFile []byte

type ARMSchemaTree map[string]*ARMSchemaEntry

type ARMSchemaEntry struct {
	Children ARMSchemaTree
	Versions []string
}

func List(ctx context.Context, subscriptionId, query string) (map[string]map[string]interface{}, error) {
	client, err := NewClient(subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	rl, err := ListTrackedResources(ctx, client, subscriptionId, query)
	if err != nil {
		return nil, err
	}

	schemaTree, err := BuildARMSchemaTree(armSchemaFile)
	if err != nil {
		return nil, err
	}

	_ = schemaTree

	out := map[string]map[string]interface{}{}
	for _, res := range rl {
		out[res.Id.String()] = res.Properties
	}
	return out, nil
}

func ListTrackedResources(ctx context.Context, client *Client, subscriptionId string, query string) ([]AzureResource, error) {
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
	return rl, nil
}

func BuildARMSchemaTree(armSchemaFile []byte) (ARMSchemaTree, error) {
	var armSchemas map[string][]string
	if err := json.Unmarshal(armSchemaFile, &armSchemas); err != nil {
		return nil, err
	}

	tree := ARMSchemaTree{}
	level := 2

	// Ensure every resoruce type starts with a provider and followed by one or more types, separated by slash(es).
	for rt := range armSchemas {
		if len(strings.Split(rt, "/")) == 1 {
			return nil, fmt.Errorf("malformed resource type: %s", rt)
		}
	}

	remains := len(armSchemas)

	for remains > 0 {
		var used []string
		for rt, versions := range armSchemas {
			// The resource types in the schema file are not consistent on casing between parent and child resources.
			upperRt := strings.ToUpper(rt)
			segs := strings.Split(upperRt, "/")
			if len(segs) == level {
				used = append(used, rt)
				entry := ARMSchemaEntry{
					Children: ARMSchemaTree{},
					Versions: versions,
				}
				tree[upperRt] = &entry
				prt := strings.Join(segs[:level-1], "/")
				if parent, ok := tree[prt]; ok {
					// Not all resource types are guaranteed to have its parent resource type defined in the arm schema,
					// that is all of the resource types defined in the arm schema are PUTable, but some parent resource types
					// might not.
					parent.Children[segs[level-1]] = &entry
				}
			}
		}

		for _, rt := range used {
			delete(armSchemas, rt)
		}

		level += 1
		remains = len(armSchemas)
	}

	return tree, nil
}
