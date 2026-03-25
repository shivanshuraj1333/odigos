import { useCallback, useState } from 'react';
import type { FlamebearerProfile, ProfilingEnableResponse } from '@/types/profiling';
import { normalizeWorkloadKindForProfiling } from '@/utils/normalizeWorkloadKindForProfiling';

function profilingUrl(namespace: string, kind: string, name: string, path: 'enable' | '') {
  const base = `/api/sources/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}/${encodeURIComponent(name)}/profiling`;
  return path === 'enable' ? `${base}/enable` : base;
}

export interface ProfilingSlotsDebug {
  activeKeys: string[];
  keysWithData: string[];
}

/** Lists in-memory profiling slots on the UI backend (same pod as GET …/profiling). */
export async function fetchProfilingSlotsDebug(): Promise<ProfilingSlotsDebug> {
  const res = await fetch('/api/debug/profiling-slots', { credentials: 'include' });
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(err.error || res.statusText);
  }
  return res.json() as Promise<ProfilingSlotsDebug>;
}

export async function enableProfilingHTTP(namespace: string, kind: string, name: string): Promise<ProfilingEnableResponse> {
  const res = await fetch(profilingUrl(namespace, kind, name, 'enable'), {
    method: 'PUT',
    credentials: 'include',
  });
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(err.error || res.statusText);
  }
  return res.json() as Promise<ProfilingEnableResponse>;
}

export async function fetchProfilingProfileHTTP(namespace: string, kind: string, name: string): Promise<FlamebearerProfile> {
  const res = await fetch(profilingUrl(namespace, kind, name, ''), {
    credentials: 'include',
  });
  if (!res.ok) {
    const err = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(err.error || res.statusText);
  }
  return res.json() as Promise<FlamebearerProfile>;
}

export interface UseProfilingHTTPState {
  loading: boolean;
  error: string | null;
  profile: FlamebearerProfile | null;
  lastSourceKey: string | null;
  /** Last successful PUT …/profiling/enable response (slot limits and canonical key). */
  enableMeta: ProfilingEnableResponse | null;
  load: (namespace: string, kind: string, name: string) => Promise<void>;
  enableAndLoad: (namespace: string, kind: string, name: string) => Promise<void>;
  clear: () => void;
}

/**
 * On-demand continuous profiling via REST (same-origin /api/sources/.../profiling).
 */
export function useProfilingHTTP(): UseProfilingHTTPState {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [profile, setProfile] = useState<FlamebearerProfile | null>(null);
  const [lastSourceKey, setLastSourceKey] = useState<string | null>(null);
  const [enableMeta, setEnableMeta] = useState<ProfilingEnableResponse | null>(null);

  const clear = useCallback(() => {
    setError(null);
    setProfile(null);
    setLastSourceKey(null);
    setEnableMeta(null);
  }, []);

  const load = useCallback(async (namespace: string, kind: string, name: string) => {
    const k = normalizeWorkloadKindForProfiling(kind);
    setLoading(true);
    setError(null);
    try {
      const p = await fetchProfilingProfileHTTP(namespace, k, name);
      setProfile(p);
      setLastSourceKey(`${namespace}/${k}/${name}`);
    } catch (e) {
      setProfile(null);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  const enableAndLoad = useCallback(async (namespace: string, kind: string, name: string) => {
    const k = normalizeWorkloadKindForProfiling(kind);
    setLoading(true);
    setError(null);
    try {
      const en = await enableProfilingHTTP(namespace, k, name);
      setEnableMeta(en);
      setLastSourceKey(en.sourceKey);
      const p = await fetchProfilingProfileHTTP(namespace, k, name);
      setProfile(p);
    } catch (e) {
      setProfile(null);
      setEnableMeta(null);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  return { loading, error, profile, lastSourceKey, enableMeta, load, enableAndLoad, clear };
}
