'use client';

import React, { useMemo, useState } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { useQuery } from '@apollo/client';
import { useProfiling } from '@/hooks';
import { GET_WORKLOADS } from '@/graphql';
import { ROUTES } from '@/utils';
import { FlexColumn, Text } from '@odigos/ui-kit/components';
import { FlamegraphRenderer, Box } from '@pyroscope/flamegraph';
import '@pyroscope/flamegraph/dist/index.css';

export default function SourcesProfilingPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const namespace = searchParams.get('namespace') ?? '';
  const kind = searchParams.get('kind') ?? '';
  const name = searchParams.get('name') ?? '';

  const [autoSync, setAutoSync] = useState(true);
  const enabled = Boolean(namespace && kind && name);
  const { profile, loading, error, refetch } = useProfiling({
    namespace,
    kind,
    name,
    enabled,
    pollingEnabled: autoSync,
  });

  const hasData = Boolean(profile?.flamebearer?.numTicks && profile?.flamebearer?.levels?.length);
  const pyroscopeProfile = useMemo(() => {
    if (!profile || !hasData) return null;
    return {
      version: profile.version ?? 1,
      flamebearer: profile.flamebearer,
      metadata: profile.metadata ?? { format: 'single', units: 'samples', name: 'cpu' },
      timeline: profile.timeline ?? null,
      groups: profile.groups ?? null,
      heatmap: profile.heatmap ?? null,
    };
  }, [profile, hasData]);
  const panelDark = { background: '#1e1e2e', borderRadius: 8, border: '1px solid #313244', padding: 16 };

  const { data: workloadsData } = useQuery<{ workloads: { id: { namespace: string; kind: string; name: string }; serviceName?: string }[] }>(GET_WORKLOADS, {
    variables: { filter: {} },
    skip: enabled,
  });
  const workloads = workloadsData?.workloads ?? [];

  if (!enabled) {
    return (
      <FlexColumn style={{ padding: 24, gap: 16, maxWidth: 800 }}>
        <Text size={18}>Profiling</Text>
        <Text family="secondary">Select a source to view CPU profiling data (eBPF node collector → gateway → this UI).</Text>
        {workloads.length > 0 ? (
          <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 8 }}>
            {workloads.map((w) => {
              const { id } = w;
              const label = id.name + (w.serviceName ? ` (${w.serviceName})` : '');
              const q = new URLSearchParams({ namespace: id.namespace, kind: id.kind, name: id.name });
              return (
                <li key={`${id.namespace}/${id.kind}/${id.name}`}>
                  <button
                    type="button"
                    onClick={() => router.push(`${ROUTES.SOURCES_PROFILING}?${q.toString()}`)}
                    style={{ padding: '10px 16px', width: '100%', textAlign: 'left', cursor: 'pointer', borderRadius: 6, border: '1px solid var(--color-border, #ddd)' }}
                  >
                    <strong>{label}</strong>
                    <span style={{ marginLeft: 8, opacity: 0.8 }}>{id.namespace} / {id.kind}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        ) : (
          <Text family="secondary">No workloads found. Ensure sources are configured in Odigos.</Text>
        )}
        <button
          type="button"
          onClick={() => router.push(ROUTES.SOURCES)}
          style={{ alignSelf: 'flex-start', padding: '8px 16px', cursor: 'pointer' }}
        >
          Back to Sources
        </button>
      </FlexColumn>
    );
  }

  const sourceLabel = `${namespace}/${kind}/${name}`;

  return (
    <FlexColumn style={{ padding: 24, gap: 16, maxWidth: 1000 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
        <button
          type="button"
          onClick={() => router.push(ROUTES.SOURCES)}
          style={{ padding: '8px 16px', cursor: 'pointer' }}
        >
          ← Back to Sources
        </button>
        <Text size={18} family="secondary">
          Profiling: {sourceLabel}
        </Text>
      </div>

      {error && (
        <div style={{ color: 'var(--color-error, #c00)', marginTop: 8 }}>
          {error}
        </div>
      )}

      {loading && !profile ? (
        <Text family="secondary">Enabling profiling and fetching data…</Text>
      ) : (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={autoSync}
                onChange={(e) => setAutoSync(e.target.checked)}
              />
              <Text size={14}>Auto sync with live data</Text>
            </label>
            {autoSync && (
              <Text size={14} family="secondary">
                (every 5 s)
              </Text>
            )}
          </div>
          <Text>
            Profiling active for this source. {profile?.flamebearer?.numTicks ?? 0} samples.
          </Text>
          <button
            type="button"
            onClick={() => refetch()}
            style={{ alignSelf: 'flex-start', padding: '8px 16px', cursor: 'pointer' }}
          >
            Refresh now
          </button>
          {hasData && pyroscopeProfile ? (
            <div style={{ marginTop: 24, width: '100%', ...panelDark }}>
              <Box>
                <FlamegraphRenderer
                  profile={pyroscopeProfile}
                  onlyDisplay="both"
                  showToolbar={true}
                  showCredit={false}
                  colorMode="dark"
                />
              </Box>
            </div>
          ) : hasData ? (
            <div style={{ marginTop: 24, width: '100%', ...panelDark, padding: 24, color: '#a6adc8' }}>
              No profile data to display.
            </div>
          ) : (
            <Text family="secondary">No profile data yet. Ensure the gateway sends profiles to the UI backend.</Text>
          )}
        </>
      )}
    </FlexColumn>
  );
}
