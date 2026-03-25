import type { NextConfig } from 'next';

// Bundle analyzer configuration
const withBundleAnalyzer = require('@next/bundle-analyzer')({
  enabled: process.env.ANALYZE === 'true',
});

const nextConfig: NextConfig = {
  transpilePackages: ['@pyroscope/flamegraph', 'graphviz-react', 'true-myth'],
  output: 'export',
  reactStrictMode: false,
  images: {
    unoptimized: true,
  },
  compiler: {
    styledComponents: true,
    // Remove console.logs in production
    removeConsole: process.env.NODE_ENV === 'production',
  },
  // Enable compression
  compress: true,
  // Enable source maps only in development
  productionBrowserSourceMaps: false,
  // Enable experimental optimizations
  experimental: {
    // Enable tree shaking for better bundle optimization
    optimizePackageImports: [
      '@odigos/ui-kit',
      '@apollo/client',
      '@apollo/experimental-nextjs-app-support',
      'graphql',
      'react',
      'react-dom',
      'react-error-boundary',
      'styled-components',
      'zustand',
      '@pyroscope/flamegraph',
    ],
  },
  // Turbopack configuration (empty config silences the warning)
  turbopack: {
    resolveAlias: {
      'styled-components': './node_modules/styled-components',
      zustand: './node_modules/zustand',
    },
  },
};

/**
 * When `yarn dev` runs on a different port than the Odigos backend (e.g. backend on :3000 via
 * kubectl port-forward, Next on :3001), set `ODIGOS_DEV_BACKEND` so `/api`, `/graphql`, etc. are
 * proxied. Only attached when NODE_ENV is `development` so `next build` (production) does not
 * warn about rewrites with `output: 'export'`.
 */
if (process.env.NODE_ENV === 'development') {
  nextConfig.rewrites = async () => {
    const backend = process.env.ODIGOS_DEV_BACKEND?.replace(/\/$/, '');
    if (!backend) return [];
    return [
      { source: '/api/:path*', destination: `${backend}/api/:path*` },
      { source: '/graphql', destination: `${backend}/graphql` },
      { source: '/auth/:path*', destination: `${backend}/auth/:path*` },
      { source: '/diagnose/:path*', destination: `${backend}/diagnose/:path*` },
      { source: '/token/:path*', destination: `${backend}/token/:path*` },
      { source: '/describe/:path*', destination: `${backend}/describe/:path*` },
      { source: '/workload/:path*', destination: `${backend}/workload/:path*` },
      { source: '/workloads', destination: `${backend}/workloads` },
      { source: '/source/:path*', destination: `${backend}/source/:path*` },
    ];
  };
}

export default withBundleAnalyzer(nextConfig);
