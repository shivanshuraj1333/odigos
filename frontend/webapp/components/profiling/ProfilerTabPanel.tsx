'use client';

import React, { useMemo, useState } from 'react';
import { FlexColumn, Text } from '@odigos/ui-kit/components';
import { useProfiling } from '@/hooks';
import { flamebearerToFlameNode } from '@/utils';
import { FlameGraph } from './FlameGraph';
import { SymbolTable } from './SymbolTable';

export interface ProfilerTabPanelProps {
  namespace: string;
  kind: string;
  name: string;
  onClose?: () => void;
}

/**
 * Panel content for the Profiler tab: "Load data" triggers enable + poll;
 * data is cached on the backend and shown here as a flame graph.
 */
type ProfilerTab = 'flame' | 'symbols';

export function ProfilerTabPanel({ namespace, kind, name, onClose }: ProfilerTabPanelProps) {
  const [loadRequested, setLoadRequested] = useState(false);
  const [autoSync, setAutoSync] = useState(true);
  const [activeTab, setActiveTab] = useState<ProfilerTab>('flame');

  const enabled = Boolean(namespace && kind && name && loadRequested);
  const { profile, loading, error, refetch } = useProfiling({
    namespace,
    kind,
    name,
    enabled,
    pollingEnabled: autoSync,
  });

  const flameTree = useMemo(() => flamebearerToFlameNode(profile), [profile]);
  const hasData = Boolean(profile?.flamebearer?.numTicks && profile?.flamebearer?.levels?.length);
  const symbols = profile?.symbols ?? [];
  const totalSamples = profile?.flamebearer?.numTicks ?? 0;
  const sourceLabel = `${namespace}/${kind}/${name}`;

  const tabStyle = (active: boolean) => ({
    padding: '8px 16px',
    cursor: 'pointer',
    border: 'none',
    borderRadius: 6,
    background: active ? '#313244' : 'transparent',
    color: active ? '#cdd6f4' : '#a6adc8',
    fontWeight: active ? 600 : 400,
    fontSize: 13,
  });
  const panelDark = { background: '#1e1e2e', borderRadius: 8, border: '1px solid #313244', padding: 16 };

  return (
    <FlexColumn style={{ padding: 24, gap: 16, maxWidth: 1000 }}>
      <Text size={16} family="secondary">
        Profiling: {sourceLabel}
      </Text>

      {!loadRequested ? (
        <>
          <Text family="secondary">
            Click &quot;Load data&quot; to start pulling profile data from the gateway into the cache, then view it here.
          </Text>
          <button
            type="button"
            onClick={() => setLoadRequested(true)}
            style={{
              alignSelf: 'flex-start',
              padding: '10px 20px',
              cursor: 'pointer',
              borderRadius: 8,
              border: '1px solid var(--color-border, #ccc)',
              background: 'var(--color-primary, #2563eb)',
              color: 'white',
              fontWeight: 600,
            }}
          >
            Load data
          </button>
        </>
      ) : (
        <>
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
                <button
                  type="button"
                  onClick={() => refetch()}
                  style={{ padding: '8px 16px', cursor: 'pointer', borderRadius: 6 }}
                >
                  Refresh now
                </button>
                <Text size={14} family="secondary">
                  {profile?.flamebearer?.numTicks ?? 0} samples
                </Text>
              </div>

              {profile?.metadata?.symbolsHint && (
                <div style={{ padding: '10px 14px', background: 'rgba(137, 180, 250, 0.15)', border: '1px solid rgba(137, 180, 250, 0.4)', borderRadius: 8, color: '#cdd6f4', fontSize: 14 }}>
                  {profile.metadata.symbolsHint}
                </div>
              )}

              {hasData ? (
                <div style={{ marginTop: 16, width: '100%', ...panelDark }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
                    <button type="button" style={tabStyle(activeTab === 'flame')} onClick={() => setActiveTab('flame')}>
                      Flame Graph
                    </button>
                    <button type="button" style={tabStyle(activeTab === 'symbols')} onClick={() => setActiveTab('symbols')}>
                      Symbols
                    </button>
                  </div>
                  {activeTab === 'flame' && (
                    <>
                      <Text size={14} family="secondary" style={{ marginBottom: 12, color: '#a6adc8', display: 'block' }}>
                        Hover for sample count and percentage.
                      </Text>
                      {flameTree ? (
                        <FlameGraph data={flameTree} width={900} height={500} dark />
                      ) : (
                        <div style={{ padding: 24, textAlign: 'center', color: '#a6adc8' }}>
                          No profile data to display.
                        </div>
                      )}
                    </>
                  )}
                  {activeTab === 'symbols' && (
                    <SymbolTable symbols={symbols} totalSamples={totalSamples} />
                  )}
                </div>
              ) : (
                <Text family="secondary">
                  No profile data yet. Ensure the gateway sends profiles to the UI backend and node collectors are running.
                </Text>
              )}
            </>
          )}
        </>
      )}

      {onClose && (
        <button
          type="button"
          onClick={onClose}
          style={{ alignSelf: 'flex-start', padding: '8px 16px', cursor: 'pointer', marginTop: 8 }}
        >
          Close
        </button>
      )}
    </FlexColumn>
  );
}
