export const ROUTES = {
  ROOT: '/',
  CHOOSE_STREAM: '/choose-stream',
  CHOOSE_SOURCES: '/choose-sources',
  CHOOSE_DESTINATION: '/choose-destination',
  SETUP_SUMMARY: '/setup-summary',
  OVERVIEW: '/overview',
  SOURCES: '/sources',
  SOURCES_PROFILING: '/sources/profiling',
  DESTINATIONS: '/destinations',
  ACTIONS: '/actions',
  INSTRUMENTATION_RULES: '/instrumentation-rules',
  SERVICE_MAP: '/service-map',
  PIPELINE_COLLECTORS: '/pipeline-collectors',
};

export const SKIP_TO_SUMMERY_QUERY_PARAM = 'skipToSummary';

const PROTOCOL = typeof window !== 'undefined' ? window.location.protocol : 'http:';
const HOSTNAME = typeof window !== 'undefined' ? window.location.hostname : '';
const PORT = typeof window !== 'undefined' ? window.location.port : '';

const IS_INGRESSED_DOMAIN = !!HOSTNAME && HOSTNAME !== 'localhost' && PORT === '';
const IS_DEV = process.env.NODE_ENV === 'development';

// In dev (Next dev server): always use relative URLs so requests go through Next proxy → backend.
// Same origin avoids CSRF/cookie and CORS issues. Rewrites use NEXT_PUBLIC_BACKEND_ORIGIN server-side.
// In prod: use explicit origin if set, else '' for same-origin (port-forward/deployed), else ingressed domain.
const EXPLICIT_BACKEND_ORIGIN = process.env.NEXT_PUBLIC_BACKEND_ORIGIN;
const BACKEND_HTTP_ORIGIN =
  typeof window !== 'undefined'
    ? (IS_DEV ? '' : (EXPLICIT_BACKEND_ORIGIN ?? (IS_INGRESSED_DOMAIN ? `${PROTOCOL}//${HOSTNAME}` : '')))
    : EXPLICIT_BACKEND_ORIGIN ?? 'http://localhost:3000';

export const API = {
  BACKEND_HTTP_ORIGIN,
  GRAPHQL: `${BACKEND_HTTP_ORIGIN}/graphql`,
  EVENTS: `${BACKEND_HTTP_ORIGIN}/api/events`,
  /** Profiling: PUT enable, GET data (poll every 5–10 s). Path params: namespace, kind, name (kind PascalCase). */
  PROFILING_ENABLE: (namespace: string, kind: string, name: string) =>
    `${BACKEND_HTTP_ORIGIN}/api/sources/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}/${encodeURIComponent(name)}/profiling/enable`,
  PROFILING_DATA: (namespace: string, kind: string, name: string) =>
    `${BACKEND_HTTP_ORIGIN}/api/sources/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}/${encodeURIComponent(name)}/profiling`,
};
