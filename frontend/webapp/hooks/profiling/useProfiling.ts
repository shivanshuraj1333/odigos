import { useCallback } from 'react';
import { useLazyQuery, useMutation } from '@apollo/client';
import { GET_PROFILING_SLOTS, GET_SOURCE_PROFILING, ENABLE_SOURCE_PROFILING, DISABLE_SOURCE_PROFILING } from '@/graphql';
import { decodeFlamebearerProfileJson } from '@/utils/profiling/decodeFlamebearerDeltas';

function logProfilingError(label: string, err: unknown) {
  console.error(`[Odigos profiling] ${label}`, err);
}

interface SourceIdentifier {
  namespace: string;
  kind: string;
  name: string;
}

// GraphQL shape — field name matches the schema (totalBytesInUse).
interface ProfilingSlotsResponse {
  activeKeys: string[];
  keysWithData: string[];
  totalBytesInUse: number;
  slotMaxBytes: number;
  maxSlots: number;
  maxTotalBytesBudget: number;
  slotTtlSeconds: number;
}

// Shape expected by @odigos/ui-kit (Profiling component uses `totalBytesUsed`).
interface ProfilingSlots {
  activeKeys: string[];
  keysWithData: string[];
  totalBytesUsed: number;
  slotMaxBytes: number;
  maxSlots: number;
  maxTotalBytesBudget: number;
  slotTtlSeconds: number;
}

interface EnableOrReleaseProfilingResult {
  status: string;
  sourceKey: string;
  maxSlots?: number;
  activeSlots?: number;
}

interface SourceProfilingResult {
  profileJson: string;
}

interface UseProfiling {
  fetchProfilingSlots: () => Promise<ProfilingSlots | undefined>;
  enableProfiling: (source: SourceIdentifier) => Promise<EnableOrReleaseProfilingResult | undefined>;
  releaseProfiling: (source: SourceIdentifier) => Promise<EnableOrReleaseProfilingResult | undefined>;
  fetchSourceProfiling: (source: SourceIdentifier) => Promise<SourceProfilingResult | undefined>;
}

export const useProfiling = (): UseProfiling => {
  const [querySlots] = useLazyQuery<{ profilingSlots: ProfilingSlotsResponse }>(GET_PROFILING_SLOTS, {
    fetchPolicy: 'network-only',
  });

  const [querySourceProfiling] = useLazyQuery<
    { computePlatform?: { source?: { profiling?: SourceProfilingResult | null } | null } | null },
    { sourceId: SourceIdentifier }
  >(GET_SOURCE_PROFILING, {
    fetchPolicy: 'network-only',
  });

  const [mutateEnable] = useMutation<{ enableSourceProfiling: EnableOrReleaseProfilingResult }, SourceIdentifier>(ENABLE_SOURCE_PROFILING, {
    onError: (e) => logProfilingError('enableSourceProfiling failed', e),
  });
  const [mutateDisable] = useMutation<{ disableSourceProfiling: EnableOrReleaseProfilingResult }, SourceIdentifier>(DISABLE_SOURCE_PROFILING, {
    onError: (e) => logProfilingError('disableSourceProfiling failed', e),
  });

  // Returns buffer/slot diagnostics. Maps totalBytesInUse (GraphQL) → totalBytesUsed (ui-kit interface).
  const fetchProfilingSlots: UseProfiling['fetchProfilingSlots'] = useCallback(async () => {
    const { data, error } = await querySlots();
    if (error) logProfilingError('profilingSlots query failed', error);
    if (!data?.profilingSlots) return undefined;
    const r = data.profilingSlots;
    return {
      activeKeys: r.activeKeys,
      keysWithData: r.keysWithData,
      totalBytesUsed: r.totalBytesInUse,
      slotMaxBytes: r.slotMaxBytes,
      maxSlots: r.maxSlots,
      maxTotalBytesBudget: r.maxTotalBytesBudget,
      slotTtlSeconds: r.slotTtlSeconds,
    };
  }, [querySlots]);

  // Activates (or refreshes) a profiling slot for a workload.
  const enableProfiling: UseProfiling['enableProfiling'] = useCallback(
    async (source) => {
      const { data, errors } = await mutateEnable({ variables: source });
      if (errors?.length) logProfilingError('enableSourceProfiling returned GraphQL errors', errors);
      return data?.enableSourceProfiling;
    },
    [mutateEnable],
  );

  // Frees an active profiling slot when the user closes the profiling panel.
  const releaseProfiling: UseProfiling['releaseProfiling'] = useCallback(
    async (source) => {
      const { data, errors } = await mutateDisable({ variables: source });
      if (errors?.length) logProfilingError('disableSourceProfiling returned GraphQL errors', errors);
      return data?.disableSourceProfiling;
    },
    [mutateDisable],
  );

  // Fetches the aggregated Pyroscope-shaped flame graph for a workload.
  const fetchSourceProfiling: UseProfiling['fetchSourceProfiling'] = useCallback(
    async (source) => {
      const { data, error } = await querySourceProfiling({ variables: { sourceId: source } });
      if (error) logProfilingError('sourceProfiling query failed', error);
      const profiling = data?.computePlatform?.source?.profiling ?? undefined;
      if (!profiling?.profileJson) {
        return profiling;
      }
      return {
        ...profiling,
        profileJson: decodeFlamebearerProfileJson(profiling.profileJson),
      };
    },
    [querySourceProfiling],
  );

  return {
    fetchProfilingSlots,
    enableProfiling,
    releaseProfiling,
    fetchSourceProfiling,
  };
};
