package loglevel

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonlogger "github.com/odigos-io/odigos/common/logger"
	odigospredicate "github.com/odigos-io/odigos/k8sutils/pkg/predicate"
	k8sutils "github.com/odigos-io/odigos/k8sutils/pkg/utils"
)

type logLevelReconciler struct {
	client.Client
	component string
}

func (r *logLevelReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	log := commonlogger.FromContext(ctx)
	cfg, err := k8sutils.GetCurrentOdigosConfiguration(ctx, r.Client)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	level := "info"
	if cfg.ComponentLogLevels != nil {
		level = cfg.ComponentLogLevels.Resolve(r.component)
	}
	log.Info("Applying log level", "component", r.component, "level", level)
	commonlogger.SetLevel(level)
	return ctrl.Result{}, nil
}

// SetupWithManager registers a controller that watches effective-config and
// dynamically updates the log level for the given component on every change.
func SetupWithManager(mgr ctrl.Manager, component string) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("loglevel-effectiveconfig").
		For(&corev1.ConfigMap{}).
		WithEventFilter(&odigospredicate.OdigosEffectiveConfigMapPredicate).
		Complete(&logLevelReconciler{Client: mgr.GetClient(), component: component})
}
