//go:build !wasm

package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	// SQLite Driver (CGO-free)
	_ "github.com/glebarez/go-sqlite"

	// PostgreSQL Driver
	_ "github.com/lib/pq"

	// MySQL Driver
	_ "github.com/go-sql-driver/mysql"

	// Oracle Driver (Pure Go)
	_ "github.com/sijms/go-ora/v2"

	// MongoDB Driver
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Database global state
var (
	dbInstance    *sql.DB
	stmtCache    = make(map[string]*sql.Stmt)
	stmtCacheKeys []string // ordered keys for LRU eviction
	stmtCacheMax  = 256    // max cached prepared statements
	stmtCacheMu  sync.RWMutex

	// MongoDB Instances
	mongoClient *mongo.Client
	mongoDB     *mongo.Database
	dbCtx       = context.Background()

	// Database query hooks
	beforeQueryHooks   []func(interface{}, interface{}) interface{}
	beforeQueryHooksMu sync.RWMutex
)

// Migrations
type Migration struct {
	Name string
	Func func()
}

var (
	migrations   []Migration
	migrationsMu sync.Mutex
)

func RegisterMigration(name string, f func()) {
	migrationsMu.Lock()
	defer migrationsMu.Unlock()
	migrations = append(migrations, Migration{Name: name, Func: f})
}

var (
	dbSchemaJSON string
	dbSchemaMu   sync.Mutex
)

func RegisterDBSchema(schemaJSON string) {
	dbSchemaMu.Lock()
	defer dbSchemaMu.Unlock()
	dbSchemaJSON = schemaJSON
}

func uploadSchemaToServStore() {
	time.Sleep(1 * time.Second)

	storeURL := os.Getenv("SERV_STORE_URL")
	if storeURL == "" {
		if discovery := os.Getenv("SERVVERSE_DISCOVERY"); discovery != "" {
			var disc map[string]string
			if err := json.Unmarshal([]byte(discovery), &disc); err == nil {
				if val, ok := disc["store"]; ok {
					storeURL = val
				}
			}
		}
	}
	if storeURL == "" {
		storeURL = "http://localhost:9000"
	}
	storeURL = strings.TrimSuffix(storeURL, "/")

	serviceName := os.Getenv("SERV_SERVICE_NAME")
	if serviceName == "" {
		serviceName = os.Getenv("SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = filepath.Base(os.Args[0])
		serviceName = strings.TrimSuffix(serviceName, ".exe")
		serviceName = strings.TrimSuffix(serviceName, "-test")
	}

	url := fmt.Sprintf("%s/console/schema?service=%s", storeURL, serviceName)
	req, err := http.NewRequest("POST", url, strings.NewReader(dbSchemaJSON))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		LogWarn("Failed to upload DB schema to ServStore: ", err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		LogInfo("Successfully registered database ORM schema to ServStore")
	} else {
		LogWarn("ServStore schema registration returned status ", resp.StatusCode)
	}
}

func RunMigrations() interface{} {
	if dbInstance == nil {
		return nil
	}

	_, err := dbInstance.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)")
	if err != nil {
		LogWarn("Failed to create schema_migrations table: ", err.Error())
		return nil
	}

	rows, err := dbInstance.Query("SELECT version FROM schema_migrations")
	if err != nil {
		LogWarn("Failed to query schema_migrations: ", err.Error())
		return nil
	}
	defer rows.Close()

	executed := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err == nil {
			executed[version] = true
		}
	}

	migrationsMu.Lock()
	defer migrationsMu.Unlock()

	for _, m := range migrations {
		if !executed[m.Name] {
			LogInfo("Running database migration: ", m.Name)
			m.Func()
			_, err := dbInstance.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Name)
			if err != nil {
				panic(fmt.Sprintf("Failed to record execution of migration %s: %s", m.Name, err.Error()))
			}
			LogInfo("Migration successful: ", m.Name)
		}
	}
	if dbSchemaJSON != "" {
		go uploadSchemaToServStore()
	}
	return nil
}

// Helper to configure connection pool from YAML config or Env
func configureDBPool(db *sql.DB) {
	maxOpen := 25
	maxIdle := 25
	lifetime := 5 * time.Minute

	if valStr := Config("database.max_open_conns"); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
			maxOpen = val
		}
	}
	if valStr := Config("database.max_idle_conns"); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
			maxIdle = val
		}
	}
	if valStr := Config("database.conn_max_lifetime"); valStr != "" {
		if dur, err := time.ParseDuration(valStr); err == nil && dur > 0 {
			lifetime = dur
		}
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(lifetime)
}

// SQLite, PostgreSQL, Oracle, and MongoDB Database Integrations
func InitDB(connStr string) {
	if strings.HasPrefix(connStr, "sqlite://") {
		dbPath := strings.TrimPrefix(connStr, "sqlite://")
		var err error
		dbInstance, err = sql.Open("sqlite", dbPath)
		if err != nil {
			panic(fmt.Sprintf("Failed to open SQLite database %s: %s", dbPath, err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to SQLite database: ", dbPath)
	} else if strings.HasPrefix(connStr, "turso://") {
		// Turso is libSQL/SQLite compatible, using glebarez/go-sqlite driver
		// turso://host:port?token=jwt or turso://endpoint
		// We translate turso:// to http:// or https:// (or let sqlite driver handle it,
		// but since go-sqlite is a pure-go SQLite driver, it doesn't do network sync natively without libsql driver.
		// However, for edge database local testing or standard SQLite fallback:
		dsn := strings.TrimPrefix(connStr, "turso://")
		if !strings.HasPrefix(dsn, "http://") && !strings.HasPrefix(dsn, "https://") {
			dsn = "https://" + dsn
		}
		var err error
		dbInstance, err = sql.Open("sqlite", dsn)
		if err != nil {
			panic(fmt.Sprintf("Failed to open Turso/libSQL database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to Turso/libSQL database successfully: ", connStr)
	} else if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		var err error
		dbInstance, err = sql.Open("postgres", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to open PostgreSQL database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to PostgreSQL database successfully")
	} else if strings.HasPrefix(connStr, "oracle://") {
		var err error
		dbInstance, err = sql.Open("oracle", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to open Oracle database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to Oracle database successfully")
	} else if strings.HasPrefix(connStr, "mysql://") {
		var err error
		dsn := strings.TrimPrefix(connStr, "mysql://")
		if strings.Contains(dsn, "@") {
			parts := strings.SplitN(dsn, "@", 2)
			creds := parts[0]
			hostDb := parts[1]
			if strings.Contains(hostDb, "/") {
				hostDbParts := strings.SplitN(hostDb, "/", 2)
				host := hostDbParts[0]
				dbName := hostDbParts[1]
				dsn = fmt.Sprintf("%s@tcp(%s)/%s", creds, host, dbName)
			}
		}
		dbInstance, err = sql.Open("mysql", dsn)
		if err != nil {
			panic(fmt.Sprintf("Failed to open MySQL database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to MySQL database successfully")
	} else if strings.HasPrefix(connStr, "mongodb://") {
		clientOptions := options.Client().ApplyURI(connStr)
		var err error
		mongoClient, err = mongo.Connect(dbCtx, clientOptions)
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to MongoDB: %s", err.Error()))
		}
		err = mongoClient.Ping(dbCtx, nil)
		if err != nil {
			LogWarn("Failed to ping MongoDB (offline/unreachable): ", err.Error())
		}
		dbName := "serv_db"
		parts := strings.Split(connStr, "/")
		if len(parts) > 3 {
			dbAndOpts := parts[len(parts)-1]
			optParts := strings.Split(dbAndOpts, "?")
			if optParts[0] != "" {
				dbName = optParts[0]
			}
		}
		mongoDB = mongoClient.Database(dbName)
		LogInfo("Connected to MongoDB successfully. Target Database: ", dbName)
	} else {
		panic(fmt.Sprintf("Unsupported database scheme in connection string: %s", connStr))
	}
}

func getCachedStmt(query string) (*sql.Stmt, error) {
	stmtCacheMu.RLock()
	stmt, exists := stmtCache[query]
	stmtCacheMu.RUnlock()
	if exists {
		// Promote to most-recently-used (move to end of keys list)
		stmtCacheMu.Lock()
		for i, k := range stmtCacheKeys {
			if k == query {
				stmtCacheKeys = append(stmtCacheKeys[:i], stmtCacheKeys[i+1:]...)
				stmtCacheKeys = append(stmtCacheKeys, query)
				break
			}
		}
		stmtCacheMu.Unlock()
		return stmt, nil
	}

	stmtCacheMu.Lock()
	defer stmtCacheMu.Unlock()
	// Double-check after acquiring write lock
	if stmt, exists = stmtCache[query]; exists {
		return stmt, nil
	}

	stmt, err := dbInstance.Prepare(query)
	if err != nil {
		return nil, err
	}

	// LRU eviction: if cache is full, close and remove the least-recently-used entry
	if len(stmtCacheKeys) >= stmtCacheMax {
		oldest := stmtCacheKeys[0]
		stmtCacheKeys = stmtCacheKeys[1:]
		if oldStmt, ok := stmtCache[oldest]; ok {
			oldStmt.Close()
			delete(stmtCache, oldest)
		}
	}

	stmtCache[query] = stmt
	stmtCacheKeys = append(stmtCacheKeys, query)
	return stmt, nil
}

func AddBeforeQueryHook(hook func(interface{}, interface{}) interface{}) {
	beforeQueryHooksMu.Lock()
	defer beforeQueryHooksMu.Unlock()
	beforeQueryHooks = append(beforeQueryHooks, hook)
}

func DBQuery(query string, args ...interface{}) interface{} {
	if mockFn, exists := GetMock("runtime.DBQuery:" + query); exists {
		return mockFn(query, args)
	}
	endSpan := TraceDB("query", query)
	defer endSpan()

	// Trigger beforeQuery hooks
	beforeQueryHooksMu.RLock()
	for _, hook := range beforeQueryHooks {
		hook(query, args)
	}
	beforeQueryHooksMu.RUnlock()
	isMongoAction := false
	if mongoDB != nil {
		q := strings.ToLower(strings.TrimSpace(query))
		if q == "find" || q == "insert" || q == "insertone" || q == "update" || q == "updateone" || q == "delete" || q == "deleteone" || q == "count" {
			isMongoAction = true
		}
	}

	if isMongoAction {
		return runMongoQuery(query, args...)
	}

	if dbInstance == nil {
		return [2]interface{}{nil, "Database is not initialized. Declare database 'sqlite://...', 'postgres://...', or 'oracle://...' first."}
	}

	stmt, err := getCachedStmt(query)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("Failed to prepare database statement: %s", err.Error())}
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.HasPrefix(queryLower, "insert") || strings.HasPrefix(queryLower, "update") ||
		strings.HasPrefix(queryLower, "delete") || strings.HasPrefix(queryLower, "create") ||
		strings.HasPrefix(queryLower, "replace") {
		res, err := stmt.ExecContext(dbCtx, args...)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("Database exec error: %s", err.Error())}
		}
		lastInsertID, _ := res.LastInsertId()
		rowsAffected, _ := res.RowsAffected()
		return map[string]interface{}{
			"last_insert_id": lastInsertID,
			"rows_affected":  rowsAffected,
		}
	}

	rows, err := stmt.QueryContext(dbCtx, args...)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("Database query error: %s", err.Error())}
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return [2]interface{}{nil, err.Error()}
	}

	var results []interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return [2]interface{}{nil, err.Error()}
		}

		row := NewSafeMap()
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row.Set(col, string(b))
			} else {
				row.Set(col, val)
			}
		}
		results = append(results, row)
	}
	return results
}

// DBQueryPage executes a paginated MongoDB find query.
// Usage: db.queryPage("collection", filter, page, pageSize)
func DBQueryPage(collection string, args ...interface{}) interface{} {
	if mockFn, exists := GetMock("runtime.DBQueryPage:" + collection); exists {
		return mockFn(collection, args)
	}
	endSpan := TraceDB("queryPage", collection)
	defer endSpan()

	if mongoDB == nil {
		return [2]interface{}{nil, "MongoDB not initialized for paginated queries"}
	}

	var filter interface{} = bson.M{}
	page := 0
	pageSize := 20

	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}
	if len(args) >= 2 {
		page = toInt(args[1])
	}
	if len(args) >= 3 {
		pageSize = toInt(args[2])
		if pageSize > 100 {
			pageSize = 100
		}
	}

	coll := mongoDB.Collection(collection)

	// Count total
	total, err := coll.CountDocuments(dbCtx, filter)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("MongoDB count error: %s", err.Error())}
	}

	// Find with skip/limit
	opts := options.Find().SetSkip(int64(page * pageSize)).SetLimit(int64(pageSize))
	cursor, err := coll.Find(dbCtx, filter, opts)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("MongoDB find error: %s", err.Error())}
	}
	defer cursor.Close(dbCtx)

	var results []interface{}
	for cursor.Next(dbCtx) {
		var row map[string]interface{}
		if err := cursor.Decode(&row); err == nil {
			results = append(results, ToSafeValue(row))
		}
	}

	return map[string]interface{}{
		"data":     results,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + int64(pageSize) - 1) / int64(pageSize),
	}
}

// DBFindOne finds a single document matching the filter.
// Usage: db.findOne("collection", filter)
func DBFindOne(collection string, args ...interface{}) interface{} {
	if mockFn, exists := GetMock("runtime.DBFindOne:" + collection); exists {
		return mockFn(collection, args)
	}
	endSpan := TraceDB("findOne", collection)
	defer endSpan()

	if mongoDB == nil {
		return [2]interface{}{nil, "MongoDB not initialized"}
	}

	var filter interface{} = bson.M{}
	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}

	coll := mongoDB.Collection(collection)
	var result map[string]interface{}
	err := coll.FindOne(dbCtx, filter).Decode(&result)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return nil
		}
		return [2]interface{}{nil, fmt.Sprintf("MongoDB findOne error: %s", err.Error())}
	}
	return ToSafeValue(result)
}

// DBCount counts documents matching a filter.
// Usage: db.count("collection", filter)
func DBCount(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("count", collection)
	defer endSpan()
	if mongoDB == nil {
		return [2]interface{}{nil, "MongoDB not initialized"}
	}

	var filter interface{} = bson.M{}
	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}

	coll := mongoDB.Collection(collection)
	count, err := coll.CountDocuments(dbCtx, filter)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("MongoDB count error: %s", err.Error())}
	}
	return count
}

// DBUpsert inserts or updates a document.
// Usage: db.upsert("collection", filter, update)
func DBUpsert(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("upsert", collection)
	defer endSpan()
	if mongoDB == nil {
		return [2]interface{}{nil, "MongoDB not initialized"}
	}
	if len(args) < 2 {
		return [2]interface{}{nil, "db.upsert requires filter and update arguments"}
	}

	var filter interface{} = bson.M{}
	var update interface{}

	// Parse filter
	filterStr, ok := args[0].(string)
	if ok {
		var f interface{}
		if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
			filter = f
		}
	} else {
		filter = args[0]
	}

	// Parse update
	updateStr, ok := args[1].(string)
	if ok {
		var u interface{}
		if err := json.Unmarshal([]byte(updateStr), &u); err == nil {
			update = u
		}
	} else {
		update = args[1]
	}

	coll := mongoDB.Collection(collection)
	opts := options.Update().SetUpsert(true)
	result, err := coll.UpdateOne(dbCtx, filter, update, opts)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("MongoDB upsert error: %s", err.Error())}
	}

	return map[string]interface{}{
		"matched_count":  result.MatchedCount,
		"modified_count": result.ModifiedCount,
		"upserted_id":   fmt.Sprint(result.UpsertedID),
	}
}

func runMongoQuery(action string, args ...interface{}) interface{} {
	if len(args) < 1 {
		return [2]interface{}{nil, "MongoDB query requires collection name as the first argument"}
	}
	collName, ok := args[0].(string)
	if !ok {
		return [2]interface{}{nil, "MongoDB collection name must be a string"}
	}

	collection := mongoDB.Collection(collName)

	var filter interface{} = bson.M{}
	if len(args) > 1 {
		filterStr, ok := args[1].(string)
		if ok {
			if strings.TrimSpace(filterStr) != "" {
				var f interface{}
				if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
					filter = f
				} else {
					filter = bson.M{"_id": filterStr}
				}
			}
		} else {
			filter = args[1]
		}
	}

	actionLower := strings.ToLower(strings.TrimSpace(action))
	switch actionLower {
	case "find":
		cursor, err := collection.Find(dbCtx, filter)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("MongoDB Find error: %s", err.Error())}
		}
		defer cursor.Close(dbCtx)
		var results []interface{}
		for cursor.Next(dbCtx) {
			var row map[string]interface{}
			if err := cursor.Decode(&row); err == nil {
				if oid, ok := row["_id"].(interface{ String() string }); ok {
					row["_id"] = oid.String()
				}
				results = append(results, ToSafeValue(row))
			}
		}
		return results

	case "insert", "insertone":
		res, err := collection.InsertOne(dbCtx, filter)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("MongoDB Insert error: %s", err.Error())}
		}
		return map[string]interface{}{
			"inserted_id": fmt.Sprint(res.InsertedID),
		}

	case "update", "updateone":
		if len(args) < 3 {
			return [2]interface{}{nil, "MongoDB update requires update document as the third argument"}
		}
		var update interface{}
		updateStr, ok := args[2].(string)
		if ok {
			var u interface{}
			if err := json.Unmarshal([]byte(updateStr), &u); err == nil {
				update = u
			} else {
				return [2]interface{}{nil, "MongoDB update document is invalid JSON"}
			}
		} else {
			update = args[2]
		}

		res, err := collection.UpdateOne(dbCtx, filter, update)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("MongoDB Update error: %s", err.Error())}
		}
		return map[string]interface{}{
			"matched_count":  res.MatchedCount,
			"modified_count": res.ModifiedCount,
		}

	case "delete", "deleteone":
		res, err := collection.DeleteOne(dbCtx, filter)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("MongoDB Delete error: %s", err.Error())}
		}
		return map[string]interface{}{
			"deleted_count": res.DeletedCount,
		}

	case "count":
		count, err := collection.CountDocuments(dbCtx, filter)
		if err != nil {
			return [2]interface{}{nil, fmt.Sprintf("MongoDB Count error: %s", err.Error())}
		}
		return count

	default:
		return [2]interface{}{nil, fmt.Sprintf("Unsupported MongoDB action: %s. Supported: find, insert, update, delete, count", action)}
	}
}

// Safe variants that return [2]interface{}{value, error} tuples for multi-return support.
// These are used when Serv code uses: let result, err = db.querySafe(...)
func DBQuerySafe(query string, args ...interface{}) interface{} {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				errVal = fmt.Sprint(rec)
			}
		}()
		result = DBQuery(query, args...)
	}()
	return [2]interface{}{result, errVal}
}

func ResetDBState() {
	if dbInstance != nil {
		rows, err := dbInstance.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%';")
		if err == nil {
			var tables []string
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err == nil {
					tables = append(tables, name)
				}
			}
			rows.Close()
			for _, table := range tables {
				dbInstance.Exec("DELETE FROM " + table + ";")
			}
		}
	}
	if mongoDB != nil {
		cols, err := mongoDB.ListCollectionNames(dbCtx, bson.D{})
		if err == nil {
			for _, col := range cols {
				mongoDB.Collection(col).Drop(dbCtx)
			}
		}
	}
}
