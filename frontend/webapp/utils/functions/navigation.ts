import { ROUTES } from '../constants';
import { SVG } from '@odigos/ui-kit/types';
import { NavbarProps } from '@odigos/ui-kit/components/v2';
import { AppRouterInstance } from 'next/dist/shared/lib/app-router-context.shared-runtime';
import { OverviewIcon, PipelineCollectorIcon, ServiceMapIcon, SettingsIcon } from '@odigos/ui-kit/icons';

const NAV_LABELS: Partial<Record<string, string>> = {
  [ROUTES.OVERVIEW]: 'Overview',
  [ROUTES.SERVICE_MAP]: 'Service map',
  [ROUTES.PIPELINE_COLLECTORS]: 'Pipeline & collectors',
  [ROUTES.SETTINGS]: 'Settings',
};

const getPayloadForIcon = (router: AppRouterInstance, currentPath: string, targetPath: string, icon: SVG): NavbarProps['icons'][number] => {
  return {
    id: targetPath,
    label: NAV_LABELS[targetPath] ?? targetPath,
    icon,
    selected: currentPath === targetPath,
    onClick: () => router.push(targetPath),
  };
};

export const getNavbarIcons = (router: AppRouterInstance, currentPath: string) => {
  const navIcons: NavbarProps['icons'] = [
    getPayloadForIcon(router, currentPath, ROUTES.OVERVIEW, OverviewIcon),
    getPayloadForIcon(router, currentPath, ROUTES.SERVICE_MAP, ServiceMapIcon),
    getPayloadForIcon(router, currentPath, ROUTES.PIPELINE_COLLECTORS, PipelineCollectorIcon),
    // getPayloadForIcon(router, currentPath, ROUTES.SAMPLING, SamplingIcon),
    getPayloadForIcon(router, currentPath, ROUTES.SETTINGS, SettingsIcon),
  ];

  return navIcons;
};
