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
	k8sclient := mgr.GetClient()

	err := ctrl.NewControllerManagedBy(mgr).
		For(&odigosv1.Action{}).
		WithEventFilter(&predicate.GenerationChangedPredicate{}).
		// Watch the shared URL-templatization Processor so that if it survives a restart
		// without any corresponding Action (orphan), we detect and clean it up on startup.
		Watches(
			&odigosv1.Processor{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() != urlTemplatizationProcessorName {
					return nil
				}
				var actionList odigosv1.ActionList
				if err := k8sclient.List(ctx, &actionList, client.InNamespace(obj.GetNamespace())); err != nil {
					return nil
				}
				var reqs []reconcile.Request
				for _, a := range actionList.Items {
					if a.Spec.URLTemplatization != nil && !a.Spec.Disabled {
						reqs = append(reqs, reconcile.Request{
							NamespacedName: types.NamespacedName{Namespace: a.Namespace, Name: a.Name},
						})
					}
				}
				// No live Actions found: Processor is a leftover. Enqueue a synthetic key
				// so the reconciler runs at least once and cleans up the Processor CR.
				if len(reqs) == 0 {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: obj.GetNamespace(),
							Name:      urlTemplatizationNamespaceSyncKey,
						},
					})
				}
				return reqs
			}),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(&ActionReconciler{Client: k8sclient})
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
