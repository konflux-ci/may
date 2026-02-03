package constants

const (
	HostLabel string = "may.konflux-ci.dev/host"

	HostControllerFinalizer string = "may.konflux-ci.dev/host-controller"

	// TenantNamespaceLabelKey is the label key used to mark namespaces as tenant (same as webhook namespaceSelector).
	TenantNamespaceLabelKey string = "konflux-ci.dev/type"
	// TenantNamespaceLabelValue is the value that marks a namespace as a tenant.
	TenantNamespaceLabelValue string = "tenant"
)
