'use client';

import { createElement, useEffect } from 'react';
import type { Source } from '@odigos/ui-kit/types';
import { SourceProfilerTab } from './SourceProfilerTab';

/**
 * Registers the Source drawer tab renderer (patch in @odigos/ui-kit calls window.__ODIGOS_SOURCE_PROFILER_TAB__).
 */
export function SourceProfilerTabRegistry() {
  useEffect(() => {
    window.__ODIGOS_SOURCE_PROFILER_TAB__ = (source: Source) => createElement(SourceProfilerTab, { source });
    return () => {
      delete window.__ODIGOS_SOURCE_PROFILER_TAB__;
    };
  }, []);
  return null;
}
