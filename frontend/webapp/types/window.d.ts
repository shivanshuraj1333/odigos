import type { Source } from '@odigos/ui-kit/types';
import type { ReactNode } from 'react';

declare global {
  interface Window {
    /** Injected by SourceProfilerTabRegistry; read by patched @odigos/ui-kit SourceDrawer. */
    __ODIGOS_SOURCE_PROFILER_TAB__?: (source: Source) => ReactNode;
  }
}

export {};
