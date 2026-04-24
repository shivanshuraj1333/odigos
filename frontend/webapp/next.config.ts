import type { NextConfig } from 'next';

// Bundle analyzer configuration
const withBundleAnalyzer = require('@next/bundle-analyzer')({
  enabled: process.env.ANALYZE === 'true',
});

const nextConfig: NextConfig = {
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
    // Exclude @odigos/ui-kit: patch-package edits deep chunk files; barrel optimization can
    // make dev bundles harder to reason about and is unnecessary for this workspace package.
    optimizePackageImports: ['@apollo/client', '@apollo/experimental-nextjs-app-support', 'graphql', 'react', 'react-dom', 'react-error-boundary', 'styled-components', 'zustand'],
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
