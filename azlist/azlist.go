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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
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

type ExtensionResource struct {
	Type   string
	Filter ResourceFilter
}

type Option struct {
	// Required
	SubscriptionId string
	Cred           azcore.TokenCredential
	ClientOpt      arm.ClientOptions

	// Optional
	Parallelism            int
	Recursive              bool
	IncludeManaged         bool
	IncludeResourceGroup   bool
	ExtensionResourceTypes []ExtensionResource
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

func List(ctx context.Context, predicate string, opt Option) (*ListResult, error) {
	if opt.Cred == nil {
		return nil, fmt.Errorf("token credential is empty")
	}
	if opt.SubscriptionId == "" {
		return nil, fmt.Errorf("subscription id is empty")
	}
	if opt.Parallelism == 0 {
		opt.Parallelism = runtime.NumCPU()
	}

	log.Info("List begins", "subscription", opt.SubscriptionId, "predicate", predicate, "parallelism", opt.Parallelism, "recursive", opt.Recursive, "include managed resources", opt.IncludeManaged)

	log.Info("New Client")
	client, err := NewClient(opt.SubscriptionId, opt.Cred, opt.ClientOpt)
	if err != nil {
		return nil, fmt.Errorf("new client: %v", err)
	}

	log.Info("Listing tracked resources")
	rl, err := ListTrackedResources(ctx, client, opt.SubscriptionId, predicate)
	if err != nil {
		return nil, err
	}

	log.Info("Build ARM schema tree")
	schemaTree, err := BuildARMSchemaTree(ARMSchemaFile)
	if err != nil {
		return nil, err
	}

	var el []ListError
	if opt.Recursive {
		log.Info("Listing child resources")
		rl, el, err = ListChildResource(ctx, client, schemaTree, rl, opt.Parallelism)
		if err != nil {
			return nil, err
		}
	}

	if !opt.IncludeManaged {
		orl := rl[:]
		rl = []AzureResource{}
		for _, res := range orl {
			if v, ok := res.Properties["managedBy"]; ok && v != "" {
				log.Info("Removing managed resource", "id", res.Id.String(), "managed by", v)
				continue
			}
			rl = append(rl, res)
		}
	}

	if opt.IncludeResourceGroup {
		rgs := map[string]AzureResource{}
		for _, res := range rl {
			root := res.Id.RootScope()
			if rg, ok := root.(*armid.ResourceGroup); ok {
				if _, ok := rgs[strings.ToUpper(rg.String())]; !ok {
					// Get the properties of the rg
					resp, err := client.resourceGroup.Get(ctx, rg.Name, nil)
					if err != nil {
						return nil, err
					}
					if resp.ID == nil {
						return nil, fmt.Errorf("unexpected nil ID of rg %s", rg.Name)
					}
					id, err := armid.ParseResourceId(*resp.ID)
					if err != nil {
						return nil, err
					}
					b, err := resp.MarshalJSON()
					if err != nil {
						return nil, err
					}
					var props map[string]interface{}
					if err := json.Unmarshal(b, &props); err != nil {
						return nil, err
					}
					rgs[strings.ToUpper(rg.String())] = AzureResource{
						Id:         id,
						Properties: props,
					}
				}
			}
		}
		rgl := []AzureResource{}
		for _, rg := range rgs {
			rgl = append(rgl, rg)
		}
		sort.Slice(rgl, func(i, j int) bool {
			return rgl[i].Id.String() < rgl[j].Id.String()
		})
		rl = append(rgl, rl...)
	}

	if len(opt.ExtensionResourceTypes) != 0 {
		log.Info("Listing extension resources")
		var extEl []ListError
		rl, extEl, err = ListExtensionResource(ctx, client, schemaTree, rl, opt.ExtensionResourceTypes, opt.Parallelism)
		if err != nil {
			return nil, err
		}
		el = append(el, extEl...)
	}

	log.Info("List ends", "list count", len(rl))

	return &ListResult{
		Resources: rl,
		Errors:    el,
	}, nil
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

	var renameRTs []string
	// Rename resource types that has trailing slash, e.g. "Microsoft.Network/publicIPAddresses/"
	for rt := range armSchemas {
		if strings.HasSuffix(rt, "/") {
			renameRTs = append(renameRTs, rt)
		}
	}
	for _, rt := range renameRTs {
		correctRT := strings.TrimSuffix(rt, "/")
		if versions, ok := armSchemas[correctRT]; !ok {
			armSchemas[correctRT] = armSchemas[rt]
		} else {
			versions = append(versions, armSchemas[rt]...)
			sort.Strings(versions)
			armSchemas[correctRT] = versions
		}
		delete(armSchemas, rt)
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
func ListChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, rl []AzureResource, parallelsim int) (outRl []AzureResource, outEl []ListError, err error) {
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
			log.Debug("Listing direct child resource", "parent", res.Id.String())
			listDirectChildResource(ctx, client, schemaTree, wp, res)
		}

		if err := wp.Done(); err != nil {
			return nil, nil, err
		}

		// Add new child resources to the resource set, also put them into the working list for new iteration.
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
	for _, res := range rset {
		outRl = append(outRl, res)
	}
	for _, le := range eset {
		outEl = append(outEl, le)
	}
	sort.Slice(outRl, func(i, j int) bool {
		return outRl[i].Id.String() < outRl[j].Id.String()
	})
	sort.Slice(outEl, func(i, j int) bool {
		return outEl[i].Endpoint < outEl[j].Endpoint
	})
	return outRl, outEl, nil
}

// ListExtensionResource will list for a list of extension resource types of each given resource, and returns the passed resource list with their child resources appended.
// Some resource type might fail to list, which will be returned in the ListError slice.
func ListExtensionResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, rl []AzureResource, rtl []ExtensionResource, parallelsim int) (outRl []AzureResource, outEl []ListError, err error) {
	if len(rtl) == 0 {
		return rl, nil, nil
	}

	if parallelsim == 0 {
		parallelsim = runtime.NumCPU()
	}

	rset := map[string]AzureResource{}
	for _, res := range rl {
		rset[strings.ToUpper(res.Id.String())] = res
	}

	eset := map[string]ListError{}

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
		log.Debug("Listing extension resource", "parent", res.Id.String())
		listExtensionResource(ctx, client, schemaTree, rtl, wp, res)
	}

	if err := wp.Done(); err != nil {
		return nil, nil, err
	}

	// Add new child resources to the resource set
	for _, res := range nrl {
		key := strings.ToUpper(res.Id.String())
		if _, ok := rset[key]; ok {
			continue
		}
		rset[key] = res
	}
	for _, le := range nel {
		key := strings.ToUpper(le.Endpoint)
		if _, ok := eset[key]; ok {
			continue
		}
		eset[key] = le
	}

	// Sort rset and eset and return
	for _, res := range rset {
		outRl = append(outRl, res)
	}
	for _, le := range eset {
		outEl = append(outEl, le)
	}
	sort.Slice(outRl, func(i, j int) bool {
		return outRl[i].Id.String() < outRl[j].Id.String()
	})
	sort.Slice(outEl, func(i, j int) bool {
		return outEl[i].Endpoint < outEl[j].Endpoint
	})
	return outRl, outEl, nil
}

// listDirectChildResource list one resource's direct child resources based on the ARM schema resource type hierarchy.
func listDirectChildResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, wp workerpool.WorkPool, res AzureResource) {
	rt := strings.ToUpper(strings.TrimLeft(res.Id.RouteScopeString(), "/"))
	schemaEntry := schemaTree[rt]
	if schemaEntry == nil {
		return
	}

	for crt, entry := range schemaEntry.Children {
		crt, entry := crt, entry
		wp.AddTask(func() (interface{}, error) {
			return listResource(ctx, client, res, crt, entry.Versions[len(entry.Versions)-1], nil)
		})
	}
	return
}

// listExtensionResource list one resource's extension resources specified.
func listExtensionResource(ctx context.Context, client *Client, schemaTree ARMSchemaTree, rtl []ExtensionResource, wp workerpool.WorkPool, res AzureResource) {
	for _, rt := range rtl {
		rt := rt
		wp.AddTask(func() (interface{}, error) {
			entry, ok := schemaTree[strings.ToUpper(rt.Type)]
			if !ok {
				return nil, fmt.Errorf("no schema entry found for resource type %s", rt.Type)
			}
			return listResource(ctx, client, res, "providers/"+rt.Type, entry.Versions[len(entry.Versions)-1], rt.Filter)
		})
	}
	return
}

type ResourceFilter func(res, extensionRes map[string]interface{}) bool

func listResource(ctx context.Context, client *Client, res AzureResource, crt, version string, filter ResourceFilter) (ListResult, error) {
	result := ListResult{
		Resources: []AzureResource{},
		Errors:    []ListError{},
	}

	pid := res.Id.String()

	addListError := func(pid, crt, apiVersion string, err error) {
		result.Errors = append(result.Errors, ListError{
			Endpoint: strings.ToUpper(pid + "/" + crt),
			Version:  apiVersion,
			Message:  err.Error(),
		})
	}
	log.Debug("Listing child resources by resource type", "parent", pid, "child resource type", crt, "api version", version)
	pager := client.resource.NewListChildPager(pid, crt, version)
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

			// Resources not meet filter are skipped
			if filter != nil && !filter(res.Properties, props) {
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
	return result, nil
}
