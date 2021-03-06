package libovsdb
// 
// import (
// 	"fmt"
// )

// NewGetSchemaArgs creates a new set of arguments for a get_schemas RPC
func NewGetSchemaArgs(schema string) []interface{} {
	return []interface{}{schema}
}

// NewTransactArgs creates a new set of arguments for a transact RPC
func NewTransactArgs(database string, operations ...Operation) []interface{} {
	dbSlice := make([]interface{}, 1)
	dbSlice[0] = database

	opsSlice := make([]interface{}, len(operations))
	for i, d := range operations {
		opsSlice[i] = d
	}
ops := append(dbSlice, opsSlice...)

	return ops
}

// NewCancelArgs creates a new set of arguments for a cancel RPC
func NewCancelArgs(id interface{}) []interface{} {
	return []interface{}{id}
}

// NewMonitorArgs creates a new set of arguments for a monitor RPC
func NewMonitorArgs(database string, value interface{}, requests map[string]MonitorRequest) []interface{} {
	return []interface{}{database, value, requests}
}

// NewMonitorCancelArgs creates a new set of arguments for a monitor_cancel RPC
func NewMonitorCancelArgs(value interface{}) []interface{} {
	return []interface{}{value}
}

// NewLockArgs creates a new set of arguments for a lock, steal or unlock RPC
func NewLockArgs(id interface{}) []interface{} {
	return []interface{}{id}
}
