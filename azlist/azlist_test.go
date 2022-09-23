package azlist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildARMSchemaTree(t *testing.T) {
	cases := []struct {
		name   string
		schema []byte
		expect func() ARMSchemaTree
	}{
		{
			name: "simple",
			schema: []byte(`{
	"Microsoft.Network/virtualnetworks": ["v1", "v2"],
	"Microsoft.Network/virtualnetworks/subnets": ["v1", "v2"],
	"Microsoft.Network/virtualnetworks/subnets/foos": ["v1", "v2"]
}`),
			expect: func() ARMSchemaTree {
				fooEntry := ARMSchemaEntry{
					Versions: []string{"v1", "v2"},
					Children: ARMSchemaTree{},
				}
				subnetEntry := ARMSchemaEntry{
					Versions: []string{"v1", "v2"},
					Children: ARMSchemaTree{
						"FOOS": &fooEntry,
					},
				}
				return ARMSchemaTree{
					"MICROSOFT.NETWORK/VIRTUALNETWORKS": &ARMSchemaEntry{
						Versions: []string{"v1", "v2"},
						Children: ARMSchemaTree{
							"SUBNETS": &subnetEntry,
						},
					},
					"MICROSOFT.NETWORK/VIRTUALNETWORKS/SUBNETS":      &subnetEntry,
					"MICROSOFT.NETWORK/VIRTUALNETWORKS/SUBNETS/FOOS": &fooEntry,
				}
			},
		},
		{
			name: "RP not starts with the first segment",
			schema: []byte(`{
	"Microsoft.ExtendedLocation/customLocations": ["v1", "v2"],
	"Microsoft.ExtendedLocation/customLocations/resourceSyncRules": ["v1", "v2"],
	"Microsoft.Capacity/resourceProviders/locations/serviceLimits": ["v1"]
}`),
			expect: func() ARMSchemaTree {
				resourceSyncRulesEntry := ARMSchemaEntry{
					Versions: []string{"v1", "v2"},
					Children: ARMSchemaTree{},
				}
				return ARMSchemaTree{
					"MICROSOFT.EXTENDEDLOCATION/CUSTOMLOCATIONS": &ARMSchemaEntry{
						Versions: []string{"v1", "v2"},
						Children: ARMSchemaTree{
							"RESOURCESYNCRULES": &resourceSyncRulesEntry,
						},
					},
					"MICROSOFT.EXTENDEDLOCATION/CUSTOMLOCATIONS/RESOURCESYNCRULES": &resourceSyncRulesEntry,
					"MICROSOFT.CAPACITY/RESOURCEPROVIDERS/LOCATIONS/SERVICELIMITS": &ARMSchemaEntry{
						Versions: []string{"v1"},
						Children: ARMSchemaTree{},
					},
				}
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := BuildARMSchemaTree(tt.schema)
			require.NoError(t, err)
			require.Equal(t, tt.expect(), tree)
		})
	}
}

func TestBuildARMSchemaTree_WithRealSchema(t *testing.T) {
	var armSchemas map[string][]string
	require.NoError(t, json.Unmarshal(ARMSchemaFile, &armSchemas))
	tree, err := BuildARMSchemaTree(ARMSchemaFile)
	require.NoError(t, err)
	require.Equal(t, len(armSchemas), len(tree))
}
