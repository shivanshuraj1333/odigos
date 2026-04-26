import { gql } from '@apollo/client';

export const GET_PROFILING_SLOTS = gql`
  query GetProfilingSlots {
    profilingSlots {
      activeKeys
      keysWithData
      totalBytesInUse
      slotMaxBytes
      maxSlots
      maxTotalBytesBudget
      slotTtlSeconds
    }
  }
`;

export const GET_SOURCE_PROFILING = gql`
  query GetSourceProfiling($sourceId: K8sSourceId!) {
    computePlatform {
      source(sourceId: $sourceId) {
        profiling {
          profileJson
        }
      }
    }
  }
`;
