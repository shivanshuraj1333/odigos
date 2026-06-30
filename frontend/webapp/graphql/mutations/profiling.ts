import { gql } from '@apollo/client';

export const ENABLE_SOURCE_PROFILING = gql`
  mutation EnableSourceProfiling($namespace: String!, $kind: String!, $name: String!) {
    enableSourceProfiling(namespace: $namespace, kind: $kind, name: $name) {
      status
      sourceKey
      maxSlots
      activeSlots
    }
  }
`;

// Flush the buffered profile chunks for a source's slot (keeps the slot open, so
// recording resumes immediately). Wired to the Profiling "Refresh" action.
export const CLEAR_SOURCE_PROFILING_BUFFER = gql`
  mutation ClearSourceProfilingBuffer($namespace: String!, $kind: String!, $name: String!) {
    clearSourceProfilingBuffer(namespace: $namespace, kind: $kind, name: $name) {
      status
      sourceKey
      activeSlots
    }
  }
`;
