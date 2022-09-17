package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildARMSchemaTree(t *testing.T) {
	var armSchemas map[string][]string
	require.NoError(t, json.Unmarshal(armSchemaFile, &armSchemas))

	tree, err := BuildARMSchemaTree(armSchemaFile)
	require.NoError(t, err)

	var count func(tree ARMSchemaTree) int
	count = func(tree ARMSchemaTree) int {
		if len(tree) == 0 {
			return 0
		}
		n := 0
		for _, entry := range tree {
			n++
			n += count(entry.Children)
		}
		return n
	}
	require.Equal(t, len(armSchemas), count(tree))
}
