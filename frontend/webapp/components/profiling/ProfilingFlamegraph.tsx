'use client';

import React, { useMemo } from 'react';
import styled from 'styled-components';
import type { FlamebearerProfile } from '@/types/profiling';
import type { ProfilerViewMode } from '@/components/profiling/profilerViewMode';
import { buildSymbolStatsRows } from './flamebearerSymbolStats';
import { FlamebearerIcicle } from './FlamebearerIcicle';
import { ProfilingSymbolTable } from './ProfilingSymbolTable';

/** Themed surface: `dropdown_bg_*` + `text.primary` track light/dark via Odigos theme. */
const GraphWrap = styled.div`
  width: 100%;
  min-height: 200px;
  overflow: auto;
  border-radius: 12px;
  border: 1px solid ${({ theme }) => theme.colors.border};
  background: ${({ theme }) => theme.colors.dropdown_bg_2 || theme.colors.dropdown_bg};
  padding: 8px;
  color: ${({ theme }) => theme.text.primary};
`;

const ContentSplit = styled.div`
  display: flex;
  flex-direction: row;
  gap: 16px;
  width: 100%;
  align-items: flex-start;
  color: ${({ theme }) => theme.text.primary};
  @media (max-width: 960px) {
    flex-direction: column;
  }
`;

const TableColumn = styled.div`
  flex: 0 0 42%;
  min-width: 260px;
  max-width: 100%;
`;

const FlameColumn = styled.div`
  flex: 1;
  min-width: 0;
  width: 100%;
`;

/**
 * Renders Odigos GET /api/sources/…/profiling JSON (Pyroscope-compatible flamebearer shape).
 * Uses a custom icicle renderer — @pyroscope/flamegraph is not bundled (incompatible with React 19 / Turbopack).
 */
export function ProfilingFlamegraph({
  profile,
  viewMode = 'both',
  search = '',
}: {
  profile: FlamebearerProfile;
  viewMode?: ProfilerViewMode;
  search?: string;
}) {
  const symbolRows = useMemo(() => buildSymbolStatsRows(profile), [profile]);

  if (!profile?.flamebearer?.names?.length) {
    return null;
  }

  if (viewMode === 'table') {
    return <ProfilingSymbolTable rows={symbolRows} search={search} />;
  }

  if (viewMode === 'flame') {
    return (
      <GraphWrap>
        <FlamebearerIcicle profile={profile} />
      </GraphWrap>
    );
  }

  return (
    <ContentSplit>
      <TableColumn>
        <ProfilingSymbolTable rows={symbolRows} search={search} />
      </TableColumn>
      <FlameColumn>
        <GraphWrap>
          <FlamebearerIcicle profile={profile} />
        </GraphWrap>
      </FlameColumn>
    </ContentSplit>
  );
}
