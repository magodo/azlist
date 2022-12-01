package azlist

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
var ARMSchemaFile []byte

type ARMSchemaTree map[string]*ARMSchemaEntry

type ARMSchemaEntry struct {
	Children ARMSchemaTree
	Versions []string
}

type Option struct {
	Parallelism    int
	Recursive      bool
	IncludeManaged bool
}

func defaultOption() *Option {
	return &Option{
		Parallelism: runtime.NumCPU(),
	}
}

type ListError struct {
	Endpoint string
	Version  string
	Message  string
}

func (e ListError) Error() string {
	return fmt.Sprintf("Listing %s (api-version=%s): %s", e.Endpoint, e.Version, e.Message)
}

type ListResult struct {
	Resources []AzureResource
	Errors    []ListError
}

func List(ctx context.Context, subscriptionId, predicate string, opt *Option) (*ListResult, error) {
	if opt == nil {
		opt = defaultOption()
	}

	client, err := NewClient(subscriptionId)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	rl, err := ListTrackedResources(ctx, client, subscriptionId, predicate)
	if err != nil {
		return nil, err
	}

	if !opt.Recursive {
		return &ListResult{
			Resources: rl,
		}, nil
	}

	schemaTree, err := BuildARMSchemaTree(ARMSchemaFile)
	if err != nil {
		return nil, err
	}

	result, err := ListChildResource(ctx, client, schemaTree, rl, opt.Parallelism)
	if err != nil {
		return nil, err
	}

	if !opt.IncludeManaged {
		orl := result.Resources[:]
		rl = []AzureResource{}
		for _, res := range orl {
			if v, ok := res.Properties["managedBy"]; ok && v != "" {
				continue
			}
			rl = append(rl, res)
		}
		result.Resources = rl
	}

	return result, nil
}

func ListTrackedResources(ctx context.Context, client *Client, subscriptionId string, predicate string) ([]AzureResource, error) {
	const top int32 = 1000

	query := fmt.Sprintf("Resources | where %s | order by id desc", predicate)
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

	sort.Slice(rl, func(i, j int) bool {
		return rl[i].Id.String() < rl[j].Id.String()
	})

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
// Some resource type might fail to list, which will be returned in the ListError slice.
func ListChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, rl []AzureResource, parallelsim int) (*ListResult, error) {
	if parallelsim == 0 {
		parallelsim = runtime.NumCPU()
	}

	rset := map[string]AzureResource{}
	for _, res := range rl {
		rset[strings.ToUpper(res.Id.String())] = res
	}

	eset := map[string]ListError{}

	for len(rl) != 0 {
		wp := workerpool.NewWorkPool(parallelsim)

		var (
			nrl []AzureResource
			nel []ListError
		)
		wp.Run(func(i interface{}) error {
			l := i.(ListResult)
			nrl = append(nrl, l.Resources...)
			nel = append(nel, l.Errors...)
			return nil
		})

		for _, res := range rl {
			res := res
			wp.AddTask(func() (interface{}, error) {
				return listDirectChildResource(ctx, client, schemaTree, res), nil
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
		for _, le := range nel {
			key := strings.ToUpper(le.Endpoint)
			if _, ok := eset[key]; ok {
				continue
			}
			eset[key] = le
		}
	}

	// Sort rset and eset and return
	var out ListResult
	for _, res := range rset {
		out.Resources = append(out.Resources, res)
	}
	for _, le := range eset {
		out.Errors = append(out.Errors, le)
	}
	sort.Slice(out.Resources, func(i, j int) bool {
		return out.Resources[i].Id.String() < out.Resources[j].Id.String()
	})
	sort.Slice(out.Errors, func(i, j int) bool {
		return out.Errors[i].Endpoint < out.Errors[j].Endpoint
	})
	return &out, nil
}

// listDirectChildResource list one resource's direct child resources based on the ARM schema resource type hierarchy.
func listDirectChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, res AzureResource) ListResult {
	pid := res.Id
	rt := strings.ToUpper(strings.TrimLeft(pid.RouteScopeString(), "/"))

	result := ListResult{
		Resources: []AzureResource{},
		Errors:    []ListError{},
	}

	addListError := func(pid armid.ResourceId, childRt, apiVersion string, err error) {
		result.Errors = append(result.Errors, ListError{
			Endpoint: strings.ToUpper(pid.String() + "/" + childRt),
			Version:  apiVersion,
			Message:  err.Error(),
		})
	}

	schemaEntry := schemaTree[rt]
	if schemaEntry == nil {
		return result
	}

	for crt, entry := range schemaEntry.Children {
		version := entry.Versions[len(entry.Versions)-1]
		pager := client.resource.NewListChildPager(pid.String(), crt, version)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				if azerr, ok := err.(*azcore.ResponseError); ok && azerr.StatusCode == http.StatusNotFound {
					// Intentionally ignore 404 on list.
					break
				}
				// For other errors, record into the list result
				addListError(pid, crt, version, err)
				break
			}
			for _, w := range page.Value {
				b, err := json.Marshal(w)
				if err != nil {
					addListError(pid, crt, version, fmt.Errorf("marshalling %v: %v", w, err))
					continue
				}
				var props map[string]interface{}
				if err := json.Unmarshal(b, &props); err != nil {
					addListError(pid, crt, version, fmt.Errorf("unmarshalling %v: %v", string(b), err))
					continue
				}
				idraw, ok := props["id"]
				if !ok {
					addListError(pid, crt, version, fmt.Errorf("no resource id found in response: %s", string(b)))
					continue
				}
				id, ok := idraw.(string)
				if !ok {
					addListError(pid, crt, version, fmt.Errorf("resource id is not a string: %s", string(b)))
					continue
				}
				azureId, err := armid.ParseResourceId(id)
				if err != nil {
					addListError(pid, crt, version, fmt.Errorf("parsing resource id %v: %v", id, err))
					continue
				}
				result.Resources = append(result.Resources, AzureResource{
					Id:         azureId,
					Properties: props,
				})
			}
		}
	}
	return result
}
