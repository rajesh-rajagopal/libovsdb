package libovsdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"

	"os"

	"github.com/cenkalti/rpc2"
	"github.com/cenkalti/rpc2/jsonrpc"
)

// OvsdbClient is an OVSDB client
type OvsdbClient struct {
	rpcClient     *rpc2.Client
	Schema        map[string]DatabaseSchema
	handlers      []NotificationHandler
	handlersMutex *sync.Mutex
}

func newOvsdbClient(c *rpc2.Client) *OvsdbClient {
	ovs := &OvsdbClient{
		rpcClient:     c,
		Schema:        make(map[string]DatabaseSchema),
		handlersMutex: &sync.Mutex{},
	}
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	if connections == nil {
		connections = make(map[*rpc2.Client]*Ovs	dbClient)
	}
	connections[c] = ovs
	fmt.Println("---newOvsdbClient----")
	fmt.Println(ovs)
	fmt.Println(connection[c])
	fmt.Println("---newOvsdbClient----")
	return ovs
}

// Would rather replace this connection map with an OvsdbClient Receiver scoped method
// Unfortunately rpc2 package acts wierd with a receiver scoped method and needs some investigation.
var connections map[*rpc2.Client]*OvsdbClient
var connectionsMutex = &sync.RWMutex{}

// DefaultAddress is the default IPV4 address that is used for a connection
const DefaultAddress = "127.0.0.1"

// DefaultPort is the default port used for a connection
const DefaultPort = 6640

// ConnectUsingProtocol creates an OVSDB connection and returns and OvsdbClient
func ConnectUsingProtocol(protocol string, target string) (*OvsdbClient, error) {
	conn, err := net.Dial(protocol, target)

fmt.Println("*****cli conn******")
	if err != nil {
		return nil, err
	}

	c := rpc2.NewClientWithCodec(jsonrpc.NewJSONCodec(conn))
	c.Handle("echo", echo)
	c.Handle("update", update)
	go c.Run()
	go handleDisconnectNotification(c)

	ovs := newOvsdbClient(c)

	// Process Async Notifications
	dbs, err := ovs.ListDbs()

	fmt.Println("---------connect db---------------")
	fmt.Println(dbs)
	fmt.Println("---------connect db---------------")

	if err == nil {
		for _, db := range dbs {
			schema, err := ovs.GetSchema(db)
			if err == nil {
				ovs.Schema[db] = *schema
			} else {
				return nil, err
			}
		}
	}
	return ovs, nil
}

// Connect creates an OVSDB connection and returns and OvsdbClient
func Connect(ipAddr string, port int) (*OvsdbClient, error) {
	if ipAddr == "" {
		ipAddr = DefaultAddress
	}

fmt.Println("-----enter------")

	if port <= 0 {
		port = DefaultPort
	}

	target := fmt.Sprintf("%s:%d", ipAddr, port)
        fmt.Println("target")
        fmt.Println(target)
        fmt.Println("target")

	return ConnectUsingProtocol("tcp", target)
}

// ConnectWithUnixSocket makes a OVSDB Connection via a Unix Socket
func ConnectWithUnixSocket(socketFile string) (*OvsdbClient, error) {

	if _, err := os.Stat(socketFile); os.IsNotExist(err) {
		return nil, errors.New("Invalid socket file")
	}

	return ConnectUsingProtocol("unix", socketFile)
}

// Register registers the supplied NotificationHandler to recieve OVSDB Notifications
func (ovs *OvsdbClient) Register(handler NotificationHandler) {
	fmt.Println("*********5**************")
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	ovs.handlers = append(ovs.handlers, handler)
}

//Get Handler by index
func getHandlerIndex(handler NotificationHandler, handlers []NotificationHandler) (int, error) {
	for i, h := range handlers {
		if reflect.DeepEqual(h, handler) {
			return i, nil
		}
	}
	return -1, errors.New("Handler not found")
}

// Unregister the supplied NotificationHandler to not recieve OVSDB Notifications anymore
func (ovs *OvsdbClient) Unregister(handler NotificationHandler) error {
	ovs.handlersMutex.Lock()
	defer ovs.handlersMutex.Unlock()
	i, err := getHandlerIndex(handler, ovs.handlers)
	if err != nil {
		return err
	}
	ovs.handlers = append(ovs.handlers[:i], ovs.handlers[i+1:]...)
	return nil
}

// NotificationHandler is the interface that must be implemented to receive notifcations
type NotificationHandler interface {
	// RFC 7047 section 4.1.6 Update Notification
	Update(context interface{}, tableUpdates TableUpdates)

	// RFC 7047 section 4.1.9 Locked Notification
	Locked([]interface{})

	// RFC 7047 section 4.1.10 Stolen Notification
	Stolen([]interface{})

	// RFC 7047 section 4.1.11 Echo Notification
	Echo([]interface{})

	Disconnected(*OvsdbClient)
}

// RFC 7047 : Section 4.1.6 : Echo
func echo(client *rpc2.Client, args []interface{}, reply *[]interface{}) error {
	*reply = args
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	if _, ok := connections[client]; ok {
		connections[client].handlersMutex.Lock()
		defer connections[client].handlersMutex.Unlock()
		for _, handler := range connections[client].handlers {
			handler.Echo(nil)
		}
	}
	return nil
}

// RFC 7047 : Update Notification Section 4.1.6
// Processing "params": [<json-value>, <table-updates>]
func update(client *rpc2.Client, params []interface{}, reply *interface{}) error {
	if len(params) < 2 {
		return errors.New("Invalid Update message")
	}
	// Ignore params[0] as we dont use the <json-value> currently for comparison

	raw, ok := params[1].(map[string]interface{})
	if !ok {
		return errors.New("Invalid Update message")
	}
	var rowUpdates map[string]map[string]RowUpdate

	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &rowUpdates)
	if err != nil {
		return err
	}

	// Update the local DB cache with the tableUpdates
	tableUpdates := getTableUpdatesFromRawUnmarshal(rowUpdates)
	connectionsMutex.RLock()
	defer connectionsMutex.RUnlock()
	if _, ok := connections[client]; ok {
		connections[client].handlersMutex.Lock()
		defer connections[client].handlersMutex.Unlock()
		for _, handler := range connections[client].handlers {
			handler.Update(params, tableUpdates)
		}
	}

	return nil
}

// GetSchema returns the schema in use for the provided database name
// RFC 7047 : get_schema
func (ovs OvsdbClient) GetSchema(dbName string) (*DatabaseSchema, error) {
	args := NewGetSchemaArgs(dbName)
	var reply DatabaseSchema
	err := ovs.rpcClient.Call("get_schema", args, &reply)
	if err != nil {
		return nil, err
	}
	ovs.Schema[dbName] = reply
	return &reply, err
}

// ListDbs returns the list of databases on the server
// RFC 7047 : list_dbs
func (ovs OvsdbClient) ListDbs() ([]string, error) {
	var dbs []string
	err := ovs.rpcClient.Call("list_dbs", nil, &dbs)
	if err != nil {
		log.Fatal("ListDbs failure", err)
	}
	return dbs, err
}

// Transact performs the provided Operation's on the database
// RFC 7047 : transact
func (ovs OvsdbClient) Transact(database string, operation ...Operation) ([]OperationResult, error) {
	fmt.Println("***********6 Transact************")
	var reply []OperationResult
	db, ok := ovs.Schema[database]
	if !ok {
		return nil, errors.New("invalid Database Schema")
	}

	if ok := db.validateOperations(operation...); !ok {
		return nil, errors.New("Validation failed for the operation")
	}

	args := NewTransactArgs(database, operation...)
	err := ovs.rpcClient.Call("transact", args, &reply)
	if err != nil {
		return nil, err
	}
	return reply, nil
}

// MonitorAll is a convenience method to monitor every table/column
func (ovs OvsdbClient) MonitorAll(database string, jsonContext interface{}) (*TableUpdates, error) {
	schema, ok := ovs.Schema[database]
	if !ok {
		return nil, errors.New("invalid Database Schema")
	}

	requests := make(map[string]MonitorRequest)
	for table, tableSchema := range schema.Tables {
		var columns []string
		for column := range tableSchema.Columns {
			columns = append(columns, column)
		}
		requests[table] = MonitorRequest{
			Columns: columns,
			Select: MonitorSelect{
				Initial: true,
				Insert:  true,
				Delete:  true,
				Modify:  true,
			}}
	}
	return ovs.Monitor(database, jsonContext, requests)
}

// Monitor will provide updates for a given table/column
// RFC 7047 : monitor
func (ovs OvsdbClient) Monitor(database string, jsonContext interface{}, requests map[string]MonitorRequest) (*TableUpdates, error) {
	var reply TableUpdates

	args := NewMonitorArgs(database, jsonContext, requests)

	// This totally sucks. Refer to golang JSON issue #6213
	var response map[string]map[string]RowUpdate
	err := ovs.rpcClient.Call("monitor", args, &response)
	reply = getTableUpdatesFromRawUnmarshal(response)
	if err != nil {
		return nil, err
	}
	return &reply, err
}

func getTableUpdatesFromRawUnmarshal(raw map[string]map[string]RowUpdate) TableUpdates {
	var tableUpdates TableUpdates
	tableUpdates.Updates = make(map[string]TableUpdate)
	for table, update := range raw {
		tableUpdate := TableUpdate{update}
		tableUpdates.Updates[table] = tableUpdate
	}
	return tableUpdates
}

func clearConnection(c *rpc2.Client) {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	if _, ok := connections[c]; ok {
		for _, handler := range connections[c].handlers {
			if handler != nil {
				handler.Disconnected(connections[c])
			}
		}
	}
	delete(connections, c)
}

func handleDisconnectNotification(c *rpc2.Client) {
	disconnected := c.DisconnectNotify()
	select {
	case <-disconnected:
		clearConnection(c)
	}
}

// Disconnect will close the OVSDB connection
func (ovs OvsdbClient) Disconnect() {
	ovs.rpcClient.Close()
	clearConnection(ovs.rpcClient)
}
