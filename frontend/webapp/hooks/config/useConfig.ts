'use client';

import { useEffect } from 'react';
import { GET_CONFIG } from '@/graphql';
import { useQuery } from '@apollo/client';
import { useNotificationStore } from '@odigos/ui-kit/store';
import { Crud, StatusType, Tier } from '@odigos/ui-kit/types';
import { InstallationStatus, type FetchedConfig } from '@/types';

export const useConfig = () => {
  const { addNotification } = useNotificationStore();

  // useQuery (not useSuspenseQuery): overview layout has no <Suspense> ancestor; missing boundary
  // surfaces as 500 Internal Server Error in next dev when the query first runs on the client.
  const { data, error } = useQuery<{ config?: FetchedConfig }>(GET_CONFIG, {
    skip: typeof window === 'undefined',
    fetchPolicy: 'cache-and-network',
  });

  useEffect(() => {
    if (error) {
      addNotification({
        type: StatusType.Error,
        title: error.name || Crud.Read,
        message: error.cause?.message || error.message,
      });
    }
  }, [error]);

  const config = data?.config;
  const isReadonly = data?.config?.readonly || false;
  const isEnterprise = (config?.tier && [Tier.Onprem].includes(config.tier)) || false;
  const installationStatus = data?.config?.installationStatus || InstallationStatus.New;

  return { config, isReadonly, isEnterprise, installationStatus };
};
