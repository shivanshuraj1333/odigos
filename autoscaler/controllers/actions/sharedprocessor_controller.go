package actions

import (
	"context"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SharedURLTemplatizationProcessorReconciler reconciles the shared URL-templatization Processor CR.
// On any change, it runs SyncUrlTemplatizationProcessor with fromActionController=false so the CR matches Actions and placement.
type SharedURLTemplatizationProcessorReconciler struct {
	client.Client
}

func (r *SharedURLTemplatizationProcessorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := commonlogger.FromContext(ctx)
	if err := SyncUrlTemplatizationProcessor(ctx, r.Client, false); err != nil {
		logger.Error(err, "Sync shared URL-templatization processor failed")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
