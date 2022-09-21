# azlist

List Azure resources and its child resources by an Azure Resource Graph [`where` predicate](https://learn.microsoft.com/en-us/azure/data-explorer/kusto/query/whereoperator).

## Example

```
azlist 'resourceGroup =~ "example-rg"'
```

## FAQ

- **Question**: What is the difference of the resource list returned by `azlist` and ARG?
    
    **Answer**: The result of `azlist` returns more than ARG. The ARG only returns ARM tracked resources, but not for the RP proxy resources (e.g. subnet, network security rules, storage containers, etc).

- **Question**: What is the difference of the resource list returned by `azlist` and ARM template export?
    
    **Answer**: They are meant to be the same. But ARM template export only support some certain falvors (e.g. resource group), while `azlist` allows more. However, `azlist` returns less information for each resource, e.g. it doesn't have the cross resource dependency.

- **Question**: Given a predicate `type =~ "microsoft.storage/storageaccounts"`, why it not only returns storage accounts, but also the child resources?
    
    **Answer**: This is by design and is mentioned in the description of `azlist`. `azlist` first calls an ARG query using the input predicate, then recursively list child resources based on the Azure resource type hierarchy. We don't really treat different `where` predicate (e.g. `resourceGruop =~ "foo"` vs `type =~ "foo"`) at this moment.



