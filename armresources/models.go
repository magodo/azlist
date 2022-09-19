package armresources

import "time"

// GenericResourceExpanded - Resource information.
type GenericResourceExpanded struct {
	// Resource extended location.
	ExtendedLocation *ExtendedLocation `json:"extendedLocation,omitempty"`

	// The identity of the resource.
	Identity *Identity `json:"identity,omitempty"`

	// The kind of the resource.
	Kind *string `json:"kind,omitempty"`

	// Resource location
	Location *string `json:"location,omitempty"`

	// ID of the resource that manages this resource.
	ManagedBy *string `json:"managedBy,omitempty"`

	// The plan of the resource.
	Plan *Plan `json:"plan,omitempty"`

	// The resource properties.
	Properties interface{} `json:"properties,omitempty"`

	// The SKU of the resource.
	SKU *SKU `json:"sku,omitempty"`

	// Resource tags
	Tags map[string]*string `json:"tags,omitempty"`

	// READ-ONLY; The changed time of the resource. This is only present if requested via the $expand query parameter.
	ChangedTime *time.Time `json:"changedTime,omitempty" azure:"ro"`

	// READ-ONLY; The created time of the resource. This is only present if requested via the $expand query parameter.
	CreatedTime *time.Time `json:"createdTime,omitempty" azure:"ro"`

	// READ-ONLY; Resource ID
	ID *string `json:"id,omitempty" azure:"ro"`

	// READ-ONLY; Resource name
	Name *string `json:"name,omitempty" azure:"ro"`

	// READ-ONLY; The provisioning state of the resource. This is only present if requested via the $expand query parameter.
	ProvisioningState *string `json:"provisioningState,omitempty" azure:"ro"`

	// READ-ONLY; Resource type
	Type *string `json:"type,omitempty" azure:"ro"`
}

// ExtendedLocation - Resource extended location.
type ExtendedLocation struct {
	// The extended location name.
	Name *string `json:"name,omitempty"`

	// The extended location type.
	Type *ExtendedLocationType `json:"type,omitempty"`
}

// Identity for the resource.
type Identity struct {
	// The identity type.
	Type *ResourceIdentityType `json:"type,omitempty"`

	// The list of user identities associated with the resource. The user identity dictionary key references will be ARM resource
	// ids in the form:
	// '/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/{identityName}'.
	UserAssignedIdentities map[string]*IdentityUserAssignedIdentitiesValue `json:"userAssignedIdentities,omitempty"`

	// READ-ONLY; The principal ID of resource identity.
	PrincipalID *string `json:"principalId,omitempty" azure:"ro"`

	// READ-ONLY; The tenant ID of resource.
	TenantID *string `json:"tenantId,omitempty" azure:"ro"`
}

type IdentityUserAssignedIdentitiesValue struct {
	// READ-ONLY; The client id of user assigned identity.
	ClientID *string `json:"clientId,omitempty" azure:"ro"`

	// READ-ONLY; The principal id of user assigned identity.
	PrincipalID *string `json:"principalId,omitempty" azure:"ro"`
}

// Plan for the resource.
type Plan struct {
	// The plan ID.
	Name *string `json:"name,omitempty"`

	// The offer ID.
	Product *string `json:"product,omitempty"`

	// The promotion code.
	PromotionCode *string `json:"promotionCode,omitempty"`

	// The publisher ID.
	Publisher *string `json:"publisher,omitempty"`

	// The plan's version.
	Version *string `json:"version,omitempty"`
}

// SKU for the resource.
type SKU struct {
	// The SKU capacity.
	Capacity *int32 `json:"capacity,omitempty"`

	// The SKU family.
	Family *string `json:"family,omitempty"`

	// The SKU model.
	Model *string `json:"model,omitempty"`

	// The SKU name.
	Name *string `json:"name,omitempty"`

	// The SKU size.
	Size *string `json:"size,omitempty"`

	// The SKU tier.
	Tier *string `json:"tier,omitempty"`
}
