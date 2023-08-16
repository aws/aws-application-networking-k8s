package k8s

const (
	// Gateway events
	GatewayEventReasonFailedAddFinalizer = "FailedAddFinalizer"
	GatewayEventReasonFailedBuildModel   = "FailedBuildModel"
	GatewayEventReasonFailedDeployModel  = "FailedDeployModel"

	// HTTPRoute events
	HTTPRouteeventReasonReconcile         = "Reconcile"
	HTTPRouteeventReasonDeploySucceed     = "DeploySucceed"
	HTTPRouteventReasonFailedAddFinalizer = "FailedAddFinalizer"
	HTTPRouteEventReasonFailedBuildModel  = "FailedBuildModel"
	HTTPRouteEventReasonFailedDeployModel = "FailedDeployModel"
	HTTPRouteEventReasonRetryReconcile    = "Retry-Reconcile"

	// GRPCRoute events
	GRPCRouteEventReasonReconcile          = "Reconcile"
	GRPCRouteEventReasonDeploySucceed      = "DeploySucceed"
	GRPCRouteEventReasonFailedAddFinalizer = "FailedAddFinalizer"
	GRPCRouteEventReasonFailedBuildModel   = "FailedBuildModel"
	GRPCRouteEventReasonFailedDeployModel  = "FailedDeployModel"
	GRPCRouteEventReasonRetryReconcile     = "Retry-Reconcile"

	// Service events
	ServiceEventReasonFailedAddFinalizer = "FailedAddFinalizer"
	ServiceEventReasonFailedBuildModel   = "FailedBuildModel"
	ServiceEventReasonFailedDeployModel  = "FailedDeployModel"

	// ServiceExport events
	ServiceExportEventReasonFailedAddFinalizer = "FailedAddFinalizer"
	ServiceExportEventReasonFailedBuildModel   = "FailedBuildModel"
	ServiceExportEventReasonFailedDeployModel  = "FailedDeployModel"

	// ServiceImport events
	ServiceImportEventReasonFailedAddFinalizer = "FailedAddFinalizer"
	ServiceImportEventReasonFailedBuildModel   = "FailedBuildModel"
	ServiceImportEventReasonFailedDeployModel  = "FailedDeployModel"
)
