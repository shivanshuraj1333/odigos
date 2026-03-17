import React from 'react';
import { useRouter } from 'next/navigation';
import { ActionDrawer, ActionModal, DestinationDrawer, DestinationModal, InstrumentationRuleDrawer, InstrumentationRuleModal, SourceDrawer, SourceModal } from '@odigos/ui-kit/containers';
import {
  useActionCRUD,
  useDescribe,
  useDestinationCategories,
  useDestinationCRUD,
  useInstrumentationRuleCRUD,
  useNamespace,
  usePotentialDestinations,
  useSourceCRUD,
  useTestConnection,
  useWorkloadUtils,
} from '@/hooks';
import { ROUTES } from '@/utils';
import { SourceDrawerProfilerTrigger } from '@/components/profiling/SourceDrawerProfilerTrigger';

const OverviewModalsAndDrawers = () => {
  const router = useRouter();
  const { fetchNamespacesWithWorkloads } = useNamespace();
  const { fetchDescribeSource } = useDescribe();
  const { testConnection } = useTestConnection();
  const { categories } = useDestinationCategories();
  const { restartWorkloads, restartPod, recoverFromRollback } = useWorkloadUtils();
  const { potentialDestinations } = usePotentialDestinations();
  const { createAction, updateAction, deleteAction } = useActionCRUD();
  const { persistSources, updateSource, fetchSourceById, fetchSourceLibraries, fetchPeerSources } = useSourceCRUD();
  const { createDestination, updateDestination, deleteDestination } = useDestinationCRUD();
  const { createInstrumentationRule, updateInstrumentationRule, deleteInstrumentationRule } = useInstrumentationRuleCRUD();

  const onViewProfiling = (source: { namespace: string; kind: string; name: string }) => {
    const params = new URLSearchParams({
      namespace: source.namespace,
      kind: source.kind,
      name: source.name,
    });
    router.push(`${ROUTES.SOURCES_PROFILING}?${params.toString()}`);
  };

  return (
    <>
      {/* modals */}
      <SourceModal fetchNamespacesWithWorkloads={fetchNamespacesWithWorkloads} persistSources={persistSources} />
      <DestinationModal
        isOnboarding={false}
        categories={categories}
        potentialDestinations={potentialDestinations}
        createDestination={createDestination}
        updateDestination={updateDestination}
        deleteDestination={deleteDestination}
        testConnection={testConnection}
      />
      <InstrumentationRuleModal createInstrumentationRule={createInstrumentationRule} />
      <ActionModal createAction={createAction} />

      {/* drawers: onViewProfiling passed for profiling nav (ui-kit types may not include it yet) */}
      <SourceDrawer
        {...({
          persistSources,
          restartWorkloads,
          restartPod,
          recoverFromRollback,
          updateSource,
          fetchSourceById,
          fetchSourceDescribe: fetchDescribeSource,
          fetchSourceLibraries,
          fetchPeerSources,
          onViewProfiling,
        } as React.ComponentProps<typeof SourceDrawer>)}
      />
      <DestinationDrawer categories={categories} updateDestination={updateDestination} deleteDestination={deleteDestination} testConnection={testConnection} />
      <InstrumentationRuleDrawer updateInstrumentationRule={updateInstrumentationRule} deleteInstrumentationRule={deleteInstrumentationRule} />
      <ActionDrawer updateAction={updateAction} deleteAction={deleteAction} />

      {/* Profiler tab: when source drawer is open, show button that opens Profiler panel (Load data → cache → flame graph) */}
      <SourceDrawerProfilerTrigger />
    </>
  );
};

export { OverviewModalsAndDrawers };
