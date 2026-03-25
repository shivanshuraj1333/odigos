'use client';

import React, { useMemo } from 'react';
import { useSearchParams } from 'next/navigation';
import styled from 'styled-components';
import { Button, FlexColumn } from '@odigos/ui-kit/components';
import { ProfilingFlamegraph } from '@/components/profiling/ProfilingFlamegraph';
import { useProfilingAutoRefresh, useProfilingHTTP } from '@/hooks/profiling';
import { TABLE_MAX_WIDTH } from '@/utils';

const Muted = styled.p`
  font-size: 0.8125rem;
  margin: 0;
`;

const Panel = styled(FlexColumn)`
  max-width: ${TABLE_MAX_WIDTH};
  width: 100%;
  gap: 16px;
  padding: 16px 0;
`;

const Row = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: flex-end;
`;

const Field = styled.label`
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 140px;
`;

const Input = styled.input`
  padding: 8px 10px;
  border-radius: 6px;
  border: 1px solid ${({ theme }) => theme.colors?.border || '#444'};
  background: ${({ theme }) => theme.colors?.secondary || '#1a1a1a'};
  color: inherit;
`;

const Title = styled.h1`
  font-size: 1.25rem;
  font-weight: 600;
  margin: 0;
`;

const Help = styled.p`
  font-size: 0.875rem;
  margin: 0;
  opacity: 0.9;
  line-height: 1.4;
`;

const KIND_OPTIONS = ['Deployment', 'StatefulSet', 'DaemonSet', 'CronJob', 'Job'] as const;

export default function ProfilingPage() {
  const searchParams = useSearchParams();
  const initialNs = searchParams.get('namespace') || searchParams.get('ns') || '';
  const initialKind = searchParams.get('kind') || 'Deployment';
  const initialName = searchParams.get('name') || '';

  const [namespace, setNamespace] = React.useState(initialNs);
  const [kind, setKind] = React.useState(initialKind);
  const [name, setName] = React.useState(initialName);

  const { loading, error, profile, lastSourceKey, enableAndLoad, load, clear } = useProfilingHTTP();

  const canSubmit = useMemo(() => !!(namespace.trim() && kind.trim() && name.trim()), [namespace, kind, name]);

  useProfilingAutoRefresh(load, namespace.trim(), kind.trim(), name.trim(), profile, { enabled: canSubmit });

  const onEnableAndLoad = async () => {
    if (!canSubmit) return;
    await enableAndLoad(namespace.trim(), kind.trim(), name.trim());
  };

  const onRefresh = async () => {
    if (!canSubmit) return;
    await load(namespace.trim(), kind.trim(), name.trim());
  };

  const ticks = profile?.flamebearer?.numTicks ?? 0;
  const emptyProfile = !profile || ticks === 0;

  return (
    <Panel>
      <FlexColumn style={{ gap: 8 }}>
        <Title>Continuous profiling</Title>
        <Help>
          Loads aggregated CPU profile data for a workload. Use the real Kubernetes{' '}
          <strong>Deployment</strong> name (e.g. <code>frontend</code> in Online Boutique), not the InstrumentationConfig resource name.
        </Help>
      </FlexColumn>

      <Row>
        <Field>
          <span>Namespace</span>
          <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} placeholder="e.g. online-boutique" autoComplete="off" />
        </Field>
        <Field>
          <span>Kind</span>
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            style={{ padding: '8px 10px', borderRadius: 6, minWidth: 140 }}
          >
            {KIND_OPTIONS.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        </Field>
        <Field>
          <span>Workload name</span>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. productcatalogservice" autoComplete="off" />
        </Field>
        <Button variant="primary" type="button" disabled={!canSubmit || loading} onClick={onEnableAndLoad}>
          {loading ? 'Loading…' : 'Enable & load profile'}
        </Button>
        <Button variant="secondary" type="button" disabled={!canSubmit || loading} onClick={onRefresh}>
          Refresh
        </Button>
        <Button variant="tertiary" type="button" onClick={clear}>
          Clear
        </Button>
      </Row>

      {lastSourceKey && (
        <Muted>
          Source key: <code>{lastSourceKey}</code>
        </Muted>
      )}

      {error && (
        <Help style={{ color: 'var(--error, #f87171)' }}>
          {error}
        </Help>
      )}

      {profile && !error && (
        <FlexColumn style={{ gap: 8 }}>
          <Help>
            Samples (ticks): {ticks.toLocaleString()} · Frames: {profile.flamebearer.names.length} · {profile.metadata?.name || 'cpu'} (
            {profile.metadata?.units || 'samples'})
          </Help>
          {profile.metadata?.symbolsHint && (
            <Muted style={{ opacity: 0.85 }}>
              {profile.metadata.symbolsHint}
            </Muted>
          )}
        </FlexColumn>
      )}

      {emptyProfile && !loading && !error && profile && (
        <Help>
          No samples yet — this page auto-refreshes until data appears. Keep traffic on the workload; first batches may lack Kubernetes labels and are dropped until the collector enriches them.
        </Help>
      )}

      {profile && !emptyProfile && <ProfilingFlamegraph profile={profile} />}
    </Panel>
  );
}
