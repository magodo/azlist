package armresources

type ClientListResponse struct {
	ResourceListResult
}

// ResourceListResult - List of resource groups.
type ResourceListResult struct {
	// An array of resources.
	Value []*GenericResourceExpanded `json:"value,omitempty"`

	// READ-ONLY; The URL to use for getting the next set of results.
	NextLink *string `json:"nextLink,omitempty" azure:"ro"`
}
