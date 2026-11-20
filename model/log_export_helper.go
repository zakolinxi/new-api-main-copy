package model

// FormatUserLogsForExport exposes the self-service log formatter to the export
// endpoint. The plain user listing hides admin-only diagnostics through
// formatUserLogs; export has to apply exactly the same filter or it would leak
// fields the list view deliberately drops.
func FormatUserLogsForExport(logs []*Log, startIdx int) {
	formatUserLogs(logs, startIdx)
}
