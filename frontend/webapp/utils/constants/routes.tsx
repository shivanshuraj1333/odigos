export const ROUTES = {
  ROOT: '/',
  ONBOARDING: '/onboarding',
  OVERVIEW: '/overview',
  SOURCES: '/sources',
  DESTINATIONS: '/destinations',
  ACTIONS: '/actions',
  INSTRUMENTATION_RULES: '/instrumentation-rules',
  SERVICE_MAP: '/service-map',
  PIPELINE_COLLECTORS: '/pipeline-collectors',
  SETTINGS: '/settings',
  SAMPLING: '/sampling',

  // legacy routes
  CHOOSE_STREAM: '/choose-stream',
  CHOOSE_SOURCES: '/choose-sources',
  CHOOSE_DESTINATION: '/choose-destination',
  SETUP_SUMMARY: '/setup-summary',
};

export const SKIP_TO_SUMMERY_QUERY_PARAM = 'skipToSummary';

const IS_DEV = process.env.NODE_ENV === 'development';
const HAS_WINDOW = typeof window !== 'undefined';
const DEFAULT = 'http://localhost:8085';

// When developing the Next app (`yarn dev`), GraphQL defaults to DEFAULT (8085) unless you set:
//   NEXT_PUBLIC_ODIGOS_BACKEND_URL=http://127.0.0.1:3000
// e.g. `yarn dev:local-ui` or `../scripts/dev-webapp-with-local-odigos-ui.sh` while odigos-ui (or
// `kubectl port-forward -n odigos-system svc/ui 3000:3000`) is running on that port.
const ENV_BACKEND = (process.env.NEXT_PUBLIC_ODIGOS_BACKEND_URL || '').replace(/\/$/, '');

const BACKEND_HTTP_ORIGIN = ENV_BACKEND
  ? ENV_BACKEND
  : IS_DEV || !HAS_WINDOW
    ? DEFAULT
    : window.location.origin;

export const API = {
  BACKEND_HTTP_ORIGIN,
  GRAPHQL: `${BACKEND_HTTP_ORIGIN}/graphql`,
  EVENTS: `${BACKEND_HTTP_ORIGIN}/api/events`,
};
