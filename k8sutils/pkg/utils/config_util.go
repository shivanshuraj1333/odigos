package utils

import (
	"context"
	"errors"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
)

// error to indicate specifically that odigos effective config is not found.
// it can be used to differentiate and react specifically to this error.
// the effective config is reconciled in the scheduler, so it is possible to have a situation where the config is not found when odigos starts.
var ErrOdigosEffectiveConfigNotFound = errors.New("odigos effective config not found")

// GetCurrentOdigosConfiguration is a helper function to get the current odigos config using a controller-runtime client
func GetCurrentOdigosConfiguration(ctx context.Context, k8sClient client.Client) (common.OdigosConfiguration, error) {
	var configMap v1.ConfigMap
	var odigosConfiguration common.OdigosConfiguration
	odigosSystemNamespaceName := env.GetCurrentNamespace()
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: odigosSystemNamespaceName, Name: consts.OdigosEffectiveConfigName},
		&configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return odigosConfiguration, ErrOdigosEffectiveConfigNotFound
		} else {
			return odigosConfiguration, err
		}
	}
	if configMap.Data == nil || configMap.Data[consts.OdigosConfigurationFileName] == "" {
		return odigosConfiguration, nil
	}
	if err := yaml.Unmarshal([]byte(configMap.Data[consts.OdigosConfigurationFileName]), &odigosConfiguration); err != nil {
		return odigosConfiguration, err
	}
	return odigosConfiguration, nil
}

// GetOdigosHelmManagedConfiguration loads the Helm-managed odigos-configuration ConfigMap
// (user-facing baseline before scheduler merges into effective-config).
func GetOdigosHelmManagedConfiguration(ctx context.Context, k8sClient client.Client) (common.OdigosConfiguration, error) {
	var configMap v1.ConfigMap
	var odigosConfiguration common.OdigosConfiguration
	ns := env.GetCurrentNamespace()
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: consts.OdigosConfigurationName}, &configMap); err != nil {
		return odigosConfiguration, err
	}
	if configMap.Data == nil || configMap.Data[consts.OdigosConfigurationFileName] == "" {
		return odigosConfiguration, nil
	}
	if err := yaml.Unmarshal([]byte(configMap.Data[consts.OdigosConfigurationFileName]), &odigosConfiguration); err != nil {
		return odigosConfiguration, err
	}
	return odigosConfiguration, nil
}

// ProfilingFromEffectiveOrHelm returns profiling settings for collector configuration.
//
// When the scheduler persists profiling into effective-config, that value wins (including
// profiling.enabled: false from overlays). If profiling is omitted from effective-config
// entirely (nil) — e.g. older scheduler binaries that drop unknown YAML keys — this falls
// back to odigos-configuration so the autoscaler still enables profiling pipelines.
//
// When effective-config includes profiling but omits gatewayUiOtlpEndpoint (common with older
// schedulers that do not persist that field), the Helm-managed odigos-configuration value is
// merged in so gateway → UI OTLP can still be overridden without upgrading the scheduler.
func ProfilingFromEffectiveOrHelm(ctx context.Context, k8sClient client.Client, effectiveProfiling *common.ProfilingConfiguration) (*common.ProfilingConfiguration, error) {
	helmCfg, helmErr := GetOdigosHelmManagedConfiguration(ctx, k8sClient)
	if helmErr != nil {
		if effectiveProfiling != nil {
			return effectiveProfiling, nil
		}
		return nil, helmErr
	}
	helmProf := helmCfg.Profiling

	if effectiveProfiling != nil {
		if effectiveProfiling.GatewayUiOtlpEndpoint == "" && helmProf != nil && helmProf.GatewayUiOtlpEndpoint != "" {
			effectiveProfiling.GatewayUiOtlpEndpoint = helmProf.GatewayUiOtlpEndpoint
		}
		return effectiveProfiling, nil
	}
	return helmProf, nil
}
