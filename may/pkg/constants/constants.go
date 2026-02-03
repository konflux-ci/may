package constants

const (
	RunnerIdLabel        string = "may.konflux-ci.dev/runner-id"
	RunnerUIDLabel       string = "may.konflux-ci.dev/runner-uid"
	RunnerTypeLabel      string = "may.konflux-ci.dev/runner-type"
	RunnerNameLabel      string = "may.konflux-ci.dev/runner"
	RunnerHookNameLabel  string = "may.konflux-ci.dev/hook"
	RunnerHookPhaseLabel string = "may.konflux-ci.dev/hook-phase"
	RunnerUserLabel      string = "may.konflux-ci.dev/runner-user"

	FieldOwnerOperator       string = "may.konflux-ci.dev"
	ClaimControllerFinalizer string = "may.konflux-ci.dev/reservation-controller"

	RunnerTypeLabelStatic  string = "static"
	RunnerTypeLabelDynamic string = "dynamic"
)
