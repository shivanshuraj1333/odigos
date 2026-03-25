'use client';

import React from 'react';
import '@pyroscope/flamegraph/dist/index.css';
import { FlamegraphRenderer } from '@pyroscope/flamegraph';
import type { FlamebearerProfile } from '@/types/profiling';

export interface ProfilingFlamegraphProps {
  profile: FlamebearerProfile;
}

/**
 * Renders backend Pyroscope-shaped profile (HTTP GET /api/sources/.../profiling body).
 */
export function ProfilingFlamegraph({ profile }: ProfilingFlamegraphProps) {
  if (!profile?.flamebearer?.names?.length) {
    return null;
  }

  return (
    <div style={{ width: '100%', minHeight: 420, overflow: 'auto' }}>
      <FlamegraphRenderer profile={profile as never} onlyDisplay="flamegraph" showToolbar />
    </div>
  );
}
