package services

import (
	"context"
	"fmt"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	commonlogger "github.com/odigos-io/odigos/common/logger"
	"github.com/odigos-io/odigos/frontend/kube"
	"github.com/odigos-io/odigos/k8sutils/pkg/env"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func SetComponentLogLevel(ctx context.Context, _ client.Client, component string, level common.OdigosLogLevel) error {
	ns := env.GetCurrentNamespace()
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm, err := kube.DefaultClient.CoreV1().ConfigMaps(ns).Get(ctx, consts.OdigosLocalUiConfigName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Create odigos-local-ui-config if missing (e.g. first time setting log level from UI).
				ownerCm, err := kube.DefaultClient.CoreV1().ConfigMaps(ns).Get(ctx, consts.OdigosConfigurationName, metav1.GetOptions{})
				if err != nil {
					return fmt.Errorf("failed to get odigos-configuration for owner reference: %w", err)
				}
				cfg := common.OdigosConfiguration{ComponentLogLevels: &common.ComponentLogLevels{}}
				setComponentLogLevelField(cfg.ComponentLogLevels, component, level)
				data, marshalErr := yaml.Marshal(cfg)
				if marshalErr != nil {
					return marshalErr
				}
				newCm := &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      consts.OdigosLocalUiConfigName,
						Namespace: ns,
						Labels:    map[string]string{k8sconsts.OdigosSystemConfigLabelKey: "local-ui"},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "v1", Kind: "ConfigMap", Name: ownerCm.Name, UID: ownerCm.UID,
						}},
					},
					Data: map[string]string{consts.OdigosConfigurationFileName: string(data)},
				}
				_, createErr := kube.DefaultClient.CoreV1().ConfigMaps(ns).Create(ctx, newCm, metav1.CreateOptions{})
				if apierrors.IsAlreadyExists(createErr) {
					// Another goroutine created the configmap concurrently; retry the loop so we update it instead.
					return apierrors.NewConflict(v1.Resource("configmaps"), consts.OdigosLocalUiConfigName, createErr)
				}
				return createErr
			}
			return err
		}
		var cfg common.OdigosConfiguration
		if cm.Data != nil && cm.Data[consts.OdigosConfigurationFileName] != "" {
			if err := yaml.Unmarshal([]byte(cm.Data[consts.OdigosConfigurationFileName]), &cfg); err != nil {
				return fmt.Errorf("parse existing config: %w", err)
			}
		}
		if cfg.ComponentLogLevels == nil {
			cfg.ComponentLogLevels = &common.ComponentLogLevels{}
		}
		setComponentLogLevelField(cfg.ComponentLogLevels, component, level)
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data[consts.OdigosConfigurationFileName] = string(data)
		_, err = kube.DefaultClient.CoreV1().ConfigMaps(ns).Update(ctx, cm, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return err
	}
	if component == "ui" || component == "" {
		commonlogger.SetLevel(string(level))
	}
	return nil
}

func setComponentLogLevelField(c *common.ComponentLogLevels, component string, level common.OdigosLogLevel) {
	if c == nil {
		return
	}
	switch component {
	case "autoscaler":
		c.Autoscaler = level
	case "scheduler":
		c.Scheduler = level
	case "instrumentor":
		c.Instrumentor = level
	case "odiglet":
		c.Odiglet = level
	case "deviceplugin":
		c.Deviceplugin = level
	case "ui":
		c.UI = level
	case "collector":
		c.Collector = level
	default:
		c.Default = level
	}
}
