import type { Destination } from '@odigos/ui-kit/types';

/** Normalize exportedSignals so ui-kit's monitors array never gets undefined (avoids .toLowerCase() on undefined). */
function normalizeExportedSignals(signals: Destination['exportedSignals']): { logs: boolean; metrics: boolean; traces: boolean } {
  if (!signals || typeof signals !== 'object') return { logs: false, metrics: false, traces: false };
  return { logs: !!signals.logs, metrics: !!signals.metrics, traces: !!signals.traces };
}

export const mapFetchedDestinations = (items: Destination[]): Destination[] => {
  return items.map((item) => {
    // Replace deprecated string values, with boolean values
    const fields =
      item.destinationType.type === 'clickhouse'
        ? item.fields.replace('"CLICKHOUSE_CREATE_SCHEME":"Create"', '"CLICKHOUSE_CREATE_SCHEME":"true"').replace('"CLICKHOUSE_CREATE_SCHEME":"Skip"', '"CLICKHOUSE_CREATE_SCHEME":"false"')
        : item.destinationType.type === 'qryn'
        ? item.fields
            .replace('"QRYN_ADD_EXPORTER_NAME":"Yes"', '"QRYN_ADD_EXPORTER_NAME":"true"')
            .replace('"QRYN_ADD_EXPORTER_NAME":"No"', '"QRYN_ADD_EXPORTER_NAME":"false"')
            .replace('"QRYN_RESOURCE_TO_TELEMETRY_CONVERSION":"Yes"', '"QRYN_RESOURCE_TO_TELEMETRY_CONVERSION":"true"')
            .replace('"QRYN_RESOURCE_TO_TELEMETRY_CONVERSION":"No"', '"QRYN_RESOURCE_TO_TELEMETRY_CONVERSION":"false"')
        : item.destinationType.type === 'qryn-oss'
        ? item.fields
            .replace('"QRYN_OSS_ADD_EXPORTER_NAME":"Yes"', '"QRYN_OSS_ADD_EXPORTER_NAME":"true"')
            .replace('"QRYN_OSS_ADD_EXPORTER_NAME":"No"', '"QRYN_OSS_ADD_EXPORTER_NAME":"false"')
            .replace('"QRYN_OSS_RESOURCE_TO_TELEMETRY_CONVERSION":"Yes"', '"QRYN_OSS_RESOURCE_TO_TELEMETRY_CONVERSION":"true"')
            .replace('"QRYN_OSS_RESOURCE_TO_TELEMETRY_CONVERSION":"No"', '"QRYN_OSS_RESOURCE_TO_TELEMETRY_CONVERSION":"false"')
        : item.fields;

    return { ...item, fields, exportedSignals: normalizeExportedSignals(item.exportedSignals) };
  });
};
