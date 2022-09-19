package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/magodo/armid"
	"github.com/magodo/workerpool"
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

type Option struct {
	Parallelism int
}

func defaultOption() *Option {
	return &Option{
		Parallelism: runtime.NumCPU(),
	}
}

func List(ctx context.Context, subscriptionId, query string, opt *Option) ([]AzureResource, error) {
	if opt == nil {
		opt = defaultOption()
	}

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

	rl, err = ListChildResource(ctx, client, schemaTree, rl, opt.Parallelism)
	if err != nil {
		return nil, err
	}

	return rl, nil
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

// ListChildResource will recursively list the direct child resources of each given resource, and returns the passed resource list with their child resources appended.
func ListChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, rl []AzureResource, parallelsim int) ([]AzureResource, error) {
	if parallelsim == 0 {
		parallelsim = runtime.NumCPU()
	}

	rset := map[string]AzureResource{}
	for _, res := range rl {
		rset[strings.ToUpper(res.Id.String())] = res
	}

	type listResult struct {
		children []AzureResource
		err      error
	}

	for len(rl) != 0 {
		wp := workerpool.NewWorkPool(parallelsim)

		var nrl []AzureResource
		wp.Run(func(i interface{}) error {
			l := i.([]AzureResource)
			nrl = append(nrl, l...)
			return nil
		})

		for _, res := range rl {
			wp.AddTask(func() (interface{}, error) {
				return listDirectChildResource(ctx, client, schemaTree, res)
			})
		}

		if err := wp.Done(); err != nil {
			return nil, err
		}

		// Add newly child resources to the resource set, also put them into the working list for new iteration.
		rl = []AzureResource{}
		for _, res := range nrl {
			key := strings.ToUpper(res.Id.String())
			if _, ok := rset[key]; ok {
				continue
			}
			rl = append(rl, res)
			rset[key] = res
		}
	}

	// Sort rset and return
	var out []AzureResource
	for _, res := range rset {
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Id.String() < out[j].Id.String()
	})
	return out, nil
}

// listDirectChildResource list one resource's direct child resources based on the ARM schema resource type hierarchy.
// If no child resource exists, a nil slice is returned.
func listDirectChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, res AzureResource) ([]AzureResource, error) {
	pid := res.Id
	rt := strings.ToUpper(strings.TrimLeft(pid.RouteScopeString(), "/"))
	var out []AzureResource
	for crt, entry := range schemaTree[rt].Children {
		version := entry.Versions[len(entry.Versions)-1]
		pager := client.resource.NewListChildPager(pid.String(), crt, version)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				if azerr, ok := err.(*azcore.ResponseError); ok && azerr.StatusCode == http.StatusNotFound {
					// Intentionally ignore 404 on list.
					break
				}
				return nil, fmt.Errorf("listing %s (api-version=%s): %v", strings.ToUpper(pid.String()+"/"+crt), version, err)
			}
			for _, w := range page.Value {
				b, err := json.Marshal(w)
				if err != nil {
					return nil, fmt.Errorf("marshalling %v: %v", w, err)
				}
				var props map[string]interface{}
				if err := json.Unmarshal(b, &props); err != nil {
					return nil, fmt.Errorf("unmarshalling %v: %v", string(b), err)
				}
				id, err := armid.ParseResourceId(props["id"].(string))
				if err != nil {
					return nil, fmt.Errorf("parsing resource id %v: %v", props["id"], err)
				}
				out = append(out, AzureResource{
					Id:         id,
					Properties: props,
				})
			}
		}
	}
	return out, nil
}
