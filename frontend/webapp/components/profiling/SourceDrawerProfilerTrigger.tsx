'use client';

import React, { useState } from 'react';
import { useDrawerStore } from '@odigos/ui-kit/store';
import { EntityTypes } from '@odigos/ui-kit/types';
import { OverviewDrawer } from '@odigos/ui-kit/containers';
import { ProfilerTabPanel } from './ProfilerTabPanel';

function isWorkloadId(id: unknown): id is { namespace: string; kind: string; name: string } {
  return (
    typeof id === 'object' &&
    id !== null &&
    'namespace' in id &&
    'kind' in id &&
    'name' in id &&
    typeof (id as { namespace: unknown }).namespace === 'string' &&
    typeof (id as { kind: unknown }).kind === 'string' &&
    typeof (id as { name: unknown }).name === 'string'
  );
}

/**
 * When the source drawer is open, show a "Profiler" button that opens the Profiler tab panel
 * (Load data → cache → flame graph). Renders beside the source drawer as a tab-like entry.
 */
export function SourceDrawerProfilerTrigger() {
  const { drawerType, drawerEntityId } = useDrawerStore();
  const [profilerOpen, setProfilerOpen] = useState(false);

  const isSourceDrawerOpen = drawerType === EntityTypes.Source;
  const workloadId = isSourceDrawerOpen && drawerEntityId && isWorkloadId(drawerEntityId) ? drawerEntityId : null;

  if (!workloadId) return null;

  return (
    <>
      {/* Tab-style button: visible when source drawer is open */}
      <button
        type="button"
        onClick={() => setProfilerOpen(true)}
        data-id="profiler-tab-trigger"
        style={{
          position: 'fixed',
          right: 24,
          bottom: 24,
          zIndex: 9998,
          padding: '10px 20px',
          cursor: 'pointer',
          borderRadius: 8,
          border: '1px solid var(--color-border, #444)',
          background: 'var(--color-bg-secondary, #2a2a2a)',
          color: 'var(--color-text-primary, #fff)',
          fontWeight: 600,
          fontSize: 14,
          boxShadow: '0 2px 8px rgba(0,0,0,0.3)',
        }}
      >
        Profiler
      </button>

      {/* Profiler panel as a drawer */}
      {profilerOpen && (
        <OverviewDrawer
          title="Profiler"
          onClose={() => setProfilerOpen(false)}
          width={960}
        >
          <ProfilerTabPanel
            namespace={workloadId.namespace}
            kind={workloadId.kind}
            name={workloadId.name}
            onClose={() => setProfilerOpen(false)}
          />
        </OverviewDrawer>
      )}
    </>
  );
}
