package policy

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type CustomHeaderPolicy struct {
	headers map[string]string
}

var _ policy.Policy = CustomHeaderPolicy{}

func (p CustomHeaderPolicy) Do(req *policy.Request) (*http.Response, error) {
	if p.headers != nil && len(p.headers) > 0 {
		for k, v := range p.headers {
			req.Raw().Header.Set(k, v)
		}
	}
	return req.Next()
}
