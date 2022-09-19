package armresources

const (
	moduleName    = "armresources"
	moduleVersion = "v1.0.0"
)

// ExtendedLocationType - The extended location type.
type ExtendedLocationType string

const (
	ExtendedLocationTypeEdgeZone ExtendedLocationType = "EdgeZone"
)

// PossibleExtendedLocationTypeValues returns the possible values for the ExtendedLocationType const type.
func PossibleExtendedLocationTypeValues() []ExtendedLocationType {
	return []ExtendedLocationType{
		ExtendedLocationTypeEdgeZone,
	}
}

// ResourceIdentityType - The identity type.
type ResourceIdentityType string

const (
	ResourceIdentityTypeSystemAssigned             ResourceIdentityType = "SystemAssigned"
	ResourceIdentityTypeUserAssigned               ResourceIdentityType = "UserAssigned"
	ResourceIdentityTypeSystemAssignedUserAssigned ResourceIdentityType = "SystemAssigned, UserAssigned"
	ResourceIdentityTypeNone                       ResourceIdentityType = "None"
)

// PossibleResourceIdentityTypeValues returns the possible values for the ResourceIdentityType const type.
func PossibleResourceIdentityTypeValues() []ResourceIdentityType {
	return []ResourceIdentityType{
		ResourceIdentityTypeSystemAssigned,
		ResourceIdentityTypeUserAssigned,
		ResourceIdentityTypeSystemAssignedUserAssigned,
		ResourceIdentityTypeNone,
	}
}
