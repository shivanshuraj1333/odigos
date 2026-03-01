package actions

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/odigos-io/odigos/api/actions/v1alpha1"
	odigosv1 "github.com/odigos-io/odigos/api/odigos/v1alpha1"
)

func SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&odigosv1.Action{}).
		WithEventFilter(&predicate.GenerationChangedPredicate{}).
		// Watch the shared URL-templatization Processor so that if it survives a restart
		// without any corresponding Action (orphan), we detect and clean it up on startup.
		Watches(
			&odigosv1.Processor{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() != urlTemplatizationProcessorName {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      urlTemplatizationProcessorName,
				}}}
			}),
		).
		Complete(&ActionReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.AddClusterInfo{}).
		Complete(&AddClusterInfoReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.DeleteAttribute{}).
		Complete(&DeleteAttributeReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.RenameAttribute{}).
		Complete(&RenameAttributeReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.ProbabilisticSampler{}).
		Complete(&ProbabilisticSamplerReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}
	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.LatencySampler{}).
		Complete(&OdigosSamplingReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.SpanAttributeSampler{}).
		Complete(&OdigosSamplingReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.ServiceNameSampler{}).
		Complete(&OdigosSamplingReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.ErrorSampler{}).
		Complete(&OdigosSamplingReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.PiiMasking{}).
		Complete(&PiiMaskingReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&v1.K8sAttributesResolver{}).
		Complete(&K8sAttributesResolverReconciler{
			Client: mgr.GetClient(),
		})
	if err != nil {
		return err
	}

	return nil
}

func RegisterWebhooks(mgr ctrl.Manager) error {
	err := builder.WebhookManagedBy(mgr).
		For(&odigosv1.Action{}).
		WithValidator(&ActionsValidator{}).
		Complete()
	if err != nil {
		return err
	}

	return nil
}
