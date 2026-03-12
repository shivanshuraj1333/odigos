package common

import "testing"

func TestComponentLogLevels_Resolve(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	_ = boolPtr // suppress unused warning if needed

	tests := []struct {
		name      string
		receiver  *ComponentLogLevels
		component string
		want      string
	}{
		// nil receiver
		{
			name:      "nil receiver returns info",
			receiver:  nil,
			component: "autoscaler",
			want:      "info",
		},
		{
			name:      "nil receiver with unknown component returns info",
			receiver:  nil,
			component: "unknown",
			want:      "info",
		},

		// empty struct
		{
			name:      "empty struct autoscaler returns info",
			receiver:  &ComponentLogLevels{},
			component: "autoscaler",
			want:      "info",
		},
		{
			name:      "empty struct scheduler returns info",
			receiver:  &ComponentLogLevels{},
			component: "scheduler",
			want:      "info",
		},
		{
			name:      "empty struct unknown component returns info",
			receiver:  &ComponentLogLevels{},
			component: "unknown-component",
			want:      "info",
		},

		// Default only set — all named components fall back to Default
		{
			name:      "default only — autoscaler falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelDebug},
			component: "autoscaler",
			want:      "debug",
		},
		{
			name:      "default only — scheduler falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn},
			component: "scheduler",
			want:      "warn",
		},
		{
			name:      "default only — instrumentor falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelError},
			component: "instrumentor",
			want:      "error",
		},
		{
			name:      "default only — odiglet falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelDebug},
			component: "odiglet",
			want:      "debug",
		},
		{
			name:      "default only — deviceplugin falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo},
			component: "deviceplugin",
			want:      "info",
		},
		{
			name:      "default only — ui falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn},
			component: "ui",
			want:      "warn",
		},
		{
			name:      "default only — collector falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelError},
			component: "collector",
			want:      "error",
		},
		{
			name:      "default only — unknown component falls back to default",
			receiver:  &ComponentLogLevels{Default: LogLevelDebug},
			component: "unknown",
			want:      "debug",
		},

		// Per-component set, Default empty → returns per-component
		{
			name:      "autoscaler set no default returns autoscaler level",
			receiver:  &ComponentLogLevels{Autoscaler: LogLevelDebug},
			component: "autoscaler",
			want:      "debug",
		},
		{
			name:      "scheduler set no default returns scheduler level",
			receiver:  &ComponentLogLevels{Scheduler: LogLevelWarn},
			component: "scheduler",
			want:      "warn",
		},
		{
			name:      "instrumentor set no default returns instrumentor level",
			receiver:  &ComponentLogLevels{Instrumentor: LogLevelError},
			component: "instrumentor",
			want:      "error",
		},
		{
			name:      "odiglet set no default returns odiglet level",
			receiver:  &ComponentLogLevels{Odiglet: LogLevelDebug},
			component: "odiglet",
			want:      "debug",
		},
		{
			name:      "deviceplugin set no default returns deviceplugin level",
			receiver:  &ComponentLogLevels{Deviceplugin: LogLevelWarn},
			component: "deviceplugin",
			want:      "warn",
		},
		{
			name:      "ui set no default returns ui level",
			receiver:  &ComponentLogLevels{UI: LogLevelError},
			component: "ui",
			want:      "error",
		},
		{
			name:      "collector set no default returns collector level",
			receiver:  &ComponentLogLevels{Collector: LogLevelDebug},
			component: "collector",
			want:      "debug",
		},

		// Per-component set AND Default set → per-component wins
		{
			name:      "autoscaler overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn, Autoscaler: LogLevelDebug},
			component: "autoscaler",
			want:      "debug",
		},
		{
			name:      "scheduler overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Scheduler: LogLevelError},
			component: "scheduler",
			want:      "error",
		},
		{
			name:      "instrumentor overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelDebug, Instrumentor: LogLevelWarn},
			component: "instrumentor",
			want:      "warn",
		},
		{
			name:      "odiglet overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelError, Odiglet: LogLevelInfo},
			component: "odiglet",
			want:      "info",
		},
		{
			name:      "deviceplugin overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn, Deviceplugin: LogLevelDebug},
			component: "deviceplugin",
			want:      "debug",
		},
		{
			name:      "ui overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, UI: LogLevelWarn},
			component: "ui",
			want:      "warn",
		},
		{
			name:      "collector overrides default",
			receiver:  &ComponentLogLevels{Default: LogLevelDebug, Collector: LogLevelError},
			component: "collector",
			want:      "error",
		},

		// Per-component wins but other components still see Default
		{
			name:      "autoscaler override does not affect scheduler which sees default",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn, Autoscaler: LogLevelDebug},
			component: "scheduler",
			want:      "warn",
		},

		// Unknown component string → falls back to Default → "info" if no default
		{
			name:      "unknown component no default returns info",
			receiver:  &ComponentLogLevels{Autoscaler: LogLevelDebug},
			component: "unknown-xyz",
			want:      "info",
		},

		// "default" component string → switch hits default: case → v="" → falls back to Default field → "info"
		{
			name:      "component string 'default' falls back to Default field then info",
			receiver:  &ComponentLogLevels{},
			component: "default",
			want:      "info",
		},
		{
			name:      "component string 'default' falls back to Default field value",
			receiver:  &ComponentLogLevels{Default: LogLevelWarn},
			component: "default",
			want:      "warn",
		},

		// All 8 component strings covered with distinct levels
		{
			name:      "all components set — autoscaler",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "autoscaler",
			want:      "error",
		},
		{
			name:      "all components set — scheduler",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "scheduler",
			want:      "warn",
		},
		{
			name:      "all components set — instrumentor",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "instrumentor",
			want:      "debug",
		},
		{
			name:      "all components set — odiglet",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "odiglet",
			want:      "error",
		},
		{
			name:      "all components set — deviceplugin",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "deviceplugin",
			want:      "warn",
		},
		{
			name:      "all components set — ui",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "ui",
			want:      "debug",
		},
		{
			name:      "all components set — collector",
			receiver:  &ComponentLogLevels{Default: LogLevelInfo, Autoscaler: LogLevelError, Scheduler: LogLevelWarn, Instrumentor: LogLevelDebug, Odiglet: LogLevelError, Deviceplugin: LogLevelWarn, UI: LogLevelDebug, Collector: LogLevelError},
			component: "collector",
			want:      "error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.receiver.Resolve(tc.component)
			if got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.component, got, tc.want)
			}
		})
	}
}
