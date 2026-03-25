'use client';

import React, { useEffect, useState } from 'react';
import styled from 'styled-components';
import type { Source } from '@odigos/ui-kit/types';
import { Button, FlexColumn, FlexRow } from '@odigos/ui-kit/components';
import { ProfilingFlamegraph } from '@/components/profiling/ProfilingFlamegraph';
import type { ProfilerViewMode } from '@/components/profiling/profilerViewMode';
import type { FlamebearerProfile } from '@/types/profiling';
import { fetchProfilingSlotsDebug, useProfilingAutoRefresh, useProfilingHTTP } from '@/hooks/profiling';

const LIVE_TOOLTIP =
  "Odigos profiler works in-memory and doesn't store any data on disk, to keep minimum memory footprint we store only last 10 minutes of data on demand";

const Panel = styled(FlexColumn)`
  width: 100%;
  gap: 12px;
  padding: 4px 0 16px;
`;

const Muted = styled.p`
  font-size: 0.8125rem;
  margin: 0;
  line-height: 1.4;
  color: ${({ theme }) => theme.text.secondary};
`;

const ErrorText = styled.p`
  font-size: 0.8125rem;
  margin: 0;
  line-height: 1.4;
  color: #f87171;
`;

const Toolbar = styled(FlexRow)`
  width: 100%;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
`;

const TitleRow = styled.div`
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 1rem;
  font-weight: 600;
`;

const SearchInput = styled.input`
  flex: 1;
  min-width: 140px;
  max-width: 320px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid ${({ theme }) => theme.colors.border};
  background: ${({ theme }) => theme.colors.dropdown_bg};
  color: ${({ theme }) => theme.text.primary};
  font-size: 13px;
  outline: none;
  &::placeholder {
    color: ${({ theme }) => theme.text.grey};
    opacity: 0.85;
  }
`;

const ModeGroup = styled(FlexRow)`
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
`;

const LiveBadge = styled.span`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: ${({ theme }) => theme.text.secondary};
  cursor: help;
  &::before {
    content: '';
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #22c55e;
    flex-shrink: 0;
  }
`;

const KeyCode = styled.code`
  font-size: 0.85em;
  color: ${({ theme }) => theme.text.primary};
  background: ${({ theme }) => theme.colors.dropdown_bg};
  padding: 2px 6px;
  border-radius: 4px;
  border: 1px solid ${({ theme }) => theme.colors.border};
`;

const DiagnosticsPre = styled.pre`
  margin: 0;
  padding: 10px;
  font-size: 11px;
  line-height: 1.4;
  border-radius: 8px;
  border: 1px solid ${({ theme }) => theme.colors.border};
  background: ${({ theme }) => theme.colors.dropdown_bg};
  color: ${({ theme }) => theme.text.primary};
  overflow: auto;
  max-height: 200px;
`;

function downloadProfileJson(profile: FlamebearerProfile, ns: string, workload: string) {
  const safe = workload.replace(/[^a-zA-Z0-9._-]+/g, '-');
  const blob = new Blob([JSON.stringify(profile, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `cpu-profile-${ns}-${safe}-${Date.now()}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

export function SourceProfilerTab({ source }: { source: Source }) {
  const ns = source.namespace;
  const kind = String(source.kind || 'Deployment');
  const name = source.name;

  const [viewMode, setViewMode] = useState<ProfilerViewMode>('both');
  const [search, setSearch] = useState('');
  const [diagLoading, setDiagLoading] = useState(false);
  const [diagError, setDiagError] = useState<string | null>(null);
  const [diagOpen, setDiagOpen] = useState(false);
  const [diagJson, setDiagJson] = useState<string | null>(null);

  const { loading, error, profile, lastSourceKey, enableMeta, enableAndLoad, load } = useProfilingHTTP();

  useEffect(() => {
    if (!ns || !name || !kind) return;
    void enableAndLoad(ns, kind, name);
  }, [ns, kind, name, enableAndLoad]);

  useProfilingAutoRefresh(load, ns, kind, name, profile, { enabled: !!(ns && name && kind) });

  const ticks = profile?.flamebearer?.numTicks ?? 0;
  const emptyProfile = !profile || ticks === 0;

  const loadDiagnostics = async () => {
    setDiagLoading(true);
    setDiagError(null);
    try {
      const d = await fetchProfilingSlotsDebug();
      setDiagJson(JSON.stringify(d, null, 2));
      setDiagOpen(true);
    } catch (e) {
      setDiagError(e instanceof Error ? e.message : String(e));
      setDiagJson(null);
    } finally {
      setDiagLoading(false);
    }
  };

  return (
    <Panel>
      <Toolbar>
        <TitleRow>CPU Profiling</TitleRow>
        <SearchInput
          type="search"
          placeholder="Search by symbol name"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search symbols"
        />
        <ModeGroup>
          {(['table', 'flame', 'both'] as const).map((m) => (
            <Button
              key={m}
              variant={viewMode === m ? 'secondary' : 'tertiary'}
              onClick={() => setViewMode(m)}
            >
              {m === 'table' ? 'Top table' : m === 'flame' ? 'Flame graph' : 'Both'}
            </Button>
          ))}
          <LiveBadge title={LIVE_TOOLTIP} aria-label={LIVE_TOOLTIP}>
            Live
          </LiveBadge>
        </ModeGroup>
      </Toolbar>

      <FlexRow style={{ flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
        <Button
          variant="secondary"
          disabled={loading || !profile || emptyProfile}
          onClick={() => profile && downloadProfileJson(profile, ns, name)}
        >
          Download snapshot
        </Button>
        <Button variant="tertiary" disabled={diagLoading} onClick={() => void loadDiagnostics()}>
          {diagLoading ? 'Loading diagnostics…' : 'Slot diagnostics'}
        </Button>
      </FlexRow>

      <FlexColumn style={{ gap: 8 }}>
        <Muted>On-demand CPU profile for this workload (same backend as REST/GraphQL profiling).</Muted>
        {lastSourceKey && (
          <Muted>
            Source key: <KeyCode>{lastSourceKey}</KeyCode>
          </Muted>
        )}
        {enableMeta && (
          <Muted>
            Profiling slots in use: {enableMeta.activeSlots} / {enableMeta.maxSlots} (in-memory; oldest evicted when full).
          </Muted>
        )}
      </FlexColumn>

      {diagError && <ErrorText>{diagError}</ErrorText>}
      {diagOpen && diagJson && (
        <FlexColumn style={{ gap: 6 }}>
          <Muted style={{ margin: 0 }}>Active keys vs keys with buffered data (UI backend):</Muted>
          <DiagnosticsPre>{diagJson}</DiagnosticsPre>
        </FlexColumn>
      )}

      <FlexColumn style={{ gap: 8, flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center' }}>
        <Button variant="secondary" disabled={loading} onClick={() => void load(ns, kind, name)}>
          {loading ? 'Loading…' : 'Refresh'}
        </Button>
      </FlexColumn>

      {error && <ErrorText>{error}</ErrorText>}

      {profile && !error && ticks > 0 && (
        <Muted>
          Total samples in view {ticks.toLocaleString()} · Frames: {profile.flamebearer.names.length} ·{' '}
          {profile.metadata?.name || 'cpu'} ({profile.metadata?.units || 'samples'})
          {profile.metadata?.symbolsHint ? ` · ${profile.metadata.symbolsHint}` : ''}
        </Muted>
      )}

      {profile && !error && emptyProfile && !loading && (
        <Muted>
          No usable CPU samples yet (auto-refresh runs while this tab is open). Send traffic to the workload; OTLP batches must include namespace/workload labels to match this source. If you see “symbols unavailable” or only 1 frame, the collector may be sending chunks without full dictionaries — try another service (e.g. productcatalogservice) or use Refresh after load.
        </Muted>
      )}

      {profile && !emptyProfile && (
        <ProfilingFlamegraph profile={profile} viewMode={viewMode} search={search} />
      )}
    </Panel>
  );
}
