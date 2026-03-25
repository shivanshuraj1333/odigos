'use client';

import { createElement, useEffect, type ReactElement } from 'react';
import type { Source } from '@odigos/ui-kit/types';
import { SourceProfilerTab } from './SourceProfilerTab';

declare global {
  interface Window {
    /** Set by patched @odigos/ui-kit SourceDrawer when the "Profiler" tab is selected. */
    __ODIGOS_SOURCE_PROFILER_TAB__?: (source: Source) => ReactElement;
  }
}

/**
 * Registers the profiler tab renderer for the Source drawer (ui-kit patch).
 * Without this, the drawer falls back to another tab's content for the Profiler tab.
 *
 * Do not add a second fixed panel — that duplicates the UI (overlay bug).
 */
export function ProfilerTabBridge() {
  useEffect(() => {
    window.__ODIGOS_SOURCE_PROFILER_TAB__ = (source: Source) =>
      createElement(SourceProfilerTab, {
        key: `${source.namespace}:${String(source.kind ?? 'Deployment')}:${source.name}`,
        source,
      });
    return () => {
      delete window.__ODIGOS_SOURCE_PROFILER_TAB__;
    };
  }, []);
  return null;
}
