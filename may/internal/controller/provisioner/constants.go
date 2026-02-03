package provisioner

const (
	RunnerHookPhaseLabelProvisioningValue string = "provisioning"
	RunnerHookPhaseLabelCleanupValue      string = "cleanup"

	RunnerTypeStatic  string = "static"
	RunnerTypeDynamic string = "dynamic"

	RunnerControllerFinalizer string = "may.konflux-ci.dev/runner-controller"
)
