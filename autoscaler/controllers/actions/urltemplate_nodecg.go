package actions

import (
	"context"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// URLTemplateNodeCGReconciler watches the node CollectorsGroup (e.g. span metrics toggled) and re-syncs
// the shared URL-templatization Processor. Uses URLTemplatizationSyncApplyFull so roles are patched
// even when the Processor CR already exists (unlike Action reconcile with URLTemplatizationSyncCreateIfMissing).
type URLTemplateNodeCGReconciler struct {
	client.Client
}

func (r *URLTemplateNodeCGReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := commonlogger.FromContext(ctx)
	if err := SyncUrlTemplatizationProcessor(ctx, r.Client, URLTemplatizationSyncApplyFull); err != nil {
		logger.Error(err, "sync URL-templatization processor after node CollectorsGroup change failed")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
