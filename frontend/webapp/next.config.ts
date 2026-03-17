import type { NextConfig } from 'next';

// Bundle analyzer configuration
const withBundleAnalyzer = require('@next/bundle-analyzer')({
  enabled: process.env.ANALYZE === 'true',
});

const nextConfig: NextConfig = {
  output: 'export',
  reactStrictMode: false,
  // Hide dev overlay/portal in development (nextjs-portal element)
  devIndicators: false,
  // Dev: proxy to backend so you can run Next (fast UI) + port-forward to cluster backend. No Go build needed.
  // In another terminal: kubectl port-forward -n <ns> svc/odigos-ui 8085:3000
  async rewrites() {
    if (process.env.NODE_ENV !== 'development') return [];
    const backend = process.env.NEXT_PUBLIC_BACKEND_ORIGIN || 'http://localhost:8085';
    return [
      { source: '/api/:path*', destination: `${backend}/api/:path*` },
      { source: '/graphql', destination: `${backend}/graphql` },
      { source: '/auth/csrf-token', destination: `${backend}/auth/csrf-token` },
      { source: '/diagnose/download', destination: `${backend}/diagnose/download` },
    ];
  },
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
    optimizePackageImports: ['@odigos/ui-kit', '@apollo/client', '@apollo/experimental-nextjs-app-support', 'graphql', 'react', 'react-dom', 'react-error-boundary', 'styled-components', 'zustand'],
  },
  // Turbopack configuration (empty config silences the warning)
  turbopack: {
    resolveAlias: {
      'styled-components': './node_modules/styled-components',
      zustand: './node_modules/zustand',
    },
  },
};

export default withBundleAnalyzer(nextConfig);
