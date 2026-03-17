'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { API } from '@/utils';
import { createCSRFHeaders, useCSRF } from '@/hooks/tokens/useCSRF';
import type { FlamebearerResponse } from '@/utils/profiling/flamebearerToFlame';

const POLL_INTERVAL_MS = 5000;

export interface UseProfilingParams {
  namespace: string;
  kind: string;
  name: string;
  enabled: boolean;
  /** When true, poll GET every 5 s for live data. Default true. */
  pollingEnabled?: boolean;
}

export interface UseProfilingResult {
  /** Flamebearer response from backend (names + levels). Use flamebearerToFlameNode(profile) for FlameGraph. */
  profile: FlamebearerResponse | null;
  loading: boolean;
  error: string | null;
  refetch: () => Promise<void>;
}

export function useProfiling({ namespace, kind, name, enabled, pollingEnabled = true }: UseProfilingParams): UseProfilingResult {
  const { token } = useCSRF();
  const [profile, setProfile] = useState<FlamebearerResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchOptions = useCallback(
    (method: 'GET' | 'PUT' = 'GET') => ({
      method,
      credentials: 'include' as RequestCredentials,
      headers: { ...createCSRFHeaders(token) },
    }),
    [token]
  );

  const fetchData = useCallback(async () => {
    if (!namespace || !kind || !name) return;
    try {
      const res = await fetch(API.PROFILING_DATA(namespace, kind, name), fetchOptions('GET'));
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        setError(body?.error || res.statusText);
        return;
      }
      const data = (await res.json()) as FlamebearerResponse;
      setProfile(data);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to fetch profile data');
    } finally {
      setLoading(false);
    }
  }, [namespace, kind, name, fetchOptions]);

  const enableOnce = useCallback(async () => {
    if (!namespace || !kind || !name) return;
    try {
      const res = await fetch(API.PROFILING_ENABLE(namespace, kind, name), fetchOptions('PUT'));
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        setError(body?.error || res.statusText);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to enable profiling');
    }
  }, [namespace, kind, name, fetchOptions]);

  useEffect(() => {
    if (!enabled || !namespace || !kind || !name) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    enableOnce().then(() => fetchData());
  }, [enabled, namespace, kind, name]);

  useEffect(() => {
    if (!enabled || !pollingEnabled || !namespace || !kind || !name) return;
    pollRef.current = setInterval(fetchData, POLL_INTERVAL_MS);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
      pollRef.current = null;
    };
  }, [enabled, pollingEnabled, namespace, kind, name, fetchData]);

  return { profile, loading, error, refetch: fetchData };
}
