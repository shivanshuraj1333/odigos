import { useEffect } from 'react';
import { useNotificationStore } from '@odigos/ui-kit/store';
import { useLazyQuery } from '@apollo/client';
import { GET_GATEWAY_INFO, GET_GATEWAY_PODS, GET_NODE_COLLECTOR_INFO, GET_NODE_COLLECTOR_PODS, GET_COLLECTOR_POD_INFO } from '@/graphql';
import {
  Crud,
  type GatewayInfo,
  StatusType,
  type NodeCollectoInfo,
  type PodInfo,
  type ExtendedPodInfo,
  type GetGatewayInfo,
  type GetGatewayPods,
  type GetNodeCollectorInfo,
  type GetNodeCollectorPods,
  type GetExtendedPodInfo,
} from '@odigos/ui-kit/types';

interface UseCollectorsResult {
  getGatewayInfo: GetGatewayInfo;
  getGatewayPods: GetGatewayPods;
  getNodeCollectorInfo: GetNodeCollectorInfo;
  getNodeCollectorPods: GetNodeCollectorPods;
  getExtendedPodInfo: GetExtendedPodInfo;
}

export const useCollectors = (): UseCollectorsResult => {
  const { addNotification } = useNotificationStore();

  const [getGatewayInfo, { error: gatewayInfoError }] = useLazyQuery<{ gatewayDeploymentInfo?: GatewayInfo }, {}>(GET_GATEWAY_INFO);
  const [getGatewayPods, { error: gatewayPodsError }] = useLazyQuery<{ gatewayPods?: PodInfo[] }, {}>(GET_GATEWAY_PODS);
  const [getNodeCollectorInfo, { error: nodeCollectorInfoError }] = useLazyQuery<{ odigletDaemonSetInfo?: NodeCollectoInfo }, {}>(GET_NODE_COLLECTOR_INFO);
  const [getNodeCollectorPods, { error: nodeCollectorPodsError }] = useLazyQuery<{ odigletPods?: PodInfo[] }, {}>(GET_NODE_COLLECTOR_PODS);
  const [getExtendedPodInfo, { error: extendedPodInfoError }] = useLazyQuery<{ collectorPod?: ExtendedPodInfo }, { namespace: string; name: string }>(GET_COLLECTOR_POD_INFO);

  useEffect(() => {
    if (gatewayInfoError) addNotification({ type: StatusType.Error, title: gatewayInfoError.name || Crud.Read, message: gatewayInfoError.cause?.message || gatewayInfoError.message });
  }, [gatewayInfoError, addNotification]);
  useEffect(() => {
    if (gatewayPodsError) addNotification({ type: StatusType.Error, title: gatewayPodsError.name || Crud.Read, message: gatewayPodsError.cause?.message || gatewayPodsError.message });
  }, [gatewayPodsError, addNotification]);
  useEffect(() => {
    if (nodeCollectorInfoError) addNotification({ type: StatusType.Error, title: nodeCollectorInfoError.name || Crud.Read, message: nodeCollectorInfoError.cause?.message || nodeCollectorInfoError.message });
  }, [nodeCollectorInfoError, addNotification]);
  useEffect(() => {
    if (nodeCollectorPodsError) addNotification({ type: StatusType.Error, title: nodeCollectorPodsError.name || Crud.Read, message: nodeCollectorPodsError.cause?.message || nodeCollectorPodsError.message });
  }, [nodeCollectorPodsError, addNotification]);
  useEffect(() => {
    if (extendedPodInfoError) addNotification({ type: StatusType.Error, title: extendedPodInfoError.name || Crud.Read, message: extendedPodInfoError.cause?.message || extendedPodInfoError.message });
  }, [extendedPodInfoError, addNotification]);

  return {
    getGatewayInfo: () => getGatewayInfo().then((result) => result.data?.gatewayDeploymentInfo),
    getGatewayPods: () => getGatewayPods().then((result) => result.data?.gatewayPods),
    getNodeCollectorInfo: () => getNodeCollectorInfo().then((result) => result.data?.odigletDaemonSetInfo),
    getNodeCollectorPods: () => getNodeCollectorPods().then((result) => result.data?.odigletPods),
    getExtendedPodInfo: (namespace: string, name: string) => getExtendedPodInfo({ variables: { namespace, name } }).then((result) => result.data?.collectorPod),
  };
};
