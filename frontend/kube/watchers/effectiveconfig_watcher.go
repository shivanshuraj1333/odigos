package watchers

import (
	"context"
	"fmt"
	"log"

	"github.com/odigos-io/odigos/common"
	"github.com/odigos-io/odigos/common/consts"
	commonlogger "github.com/odigos-io/odigos/common/logger"
	"github.com/odigos-io/odigos/frontend/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/yaml"
)

// StartEffectiveConfigWatcher watches the effective-config ConfigMap and dynamically
// updates the UI log level whenever the effective configuration changes.
func StartEffectiveConfigWatcher(ctx context.Context, namespace string) error {
	fieldSelector := "metadata.name=" + consts.OdigosEffectiveConfigName

	watcher, err := StartRetryWatcher(ctx, WatcherConfig[*corev1.ConfigMapList]{
		ListFunc: func(ctx context.Context, opts metav1.ListOptions) (*corev1.ConfigMapList, error) {
			opts.FieldSelector = fieldSelector
			return kube.DefaultClient.CoreV1().ConfigMaps(namespace).List(ctx, opts)
		},
		WatchFunc: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return kube.DefaultClient.CoreV1().ConfigMaps(namespace).Watch(ctx, opts)
		},
		GetResourceVersion: func(list *corev1.ConfigMapList) string {
			return list.ResourceVersion
		},
		ResourceName: "effective-config",
	})
	if err != nil {
		return fmt.Errorf("error starting effective-config watcher: %w", err)
	}

	go handleEffectiveConfigWatchEvents(ctx, watcher)
	return nil
}

func handleEffectiveConfigWatchEvents(ctx context.Context, watcher watch.Interface) {
	ch := watcher.ResultChan()
	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			return
		case event, ok := <-ch:
			if !ok {
				log.Println("Effective-config watcher closed")
				return
			}
			if event.Type == watch.Added || event.Type == watch.Modified {
				cm, ok := event.Object.(*corev1.ConfigMap)
				if !ok {
					continue
				}
				applyLogLevelFromEffectiveConfig(cm)
			}
		}
	}
}

func applyLogLevelFromEffectiveConfig(cm *corev1.ConfigMap) {
	if cm.Data == nil || cm.Data[consts.OdigosConfigurationFileName] == "" {
		return
	}
	var cfg common.OdigosConfiguration
	if err := yaml.Unmarshal([]byte(cm.Data[consts.OdigosConfigurationFileName]), &cfg); err != nil {
		return
	}
	if cfg.ComponentLogLevels == nil {
		return
	}
	commonlogger.SetLevel(cfg.ComponentLogLevels.Resolve("ui"))
}
