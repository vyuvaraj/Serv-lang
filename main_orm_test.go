package main

import (
	"os"
	"testing"
	"serv/compiler"
)

func TestORMUnitParser(t *testing.T) {
	input := `
migration "20260616_test" {
	db.query("CREATE TABLE users (id INTEGER, name TEXT, balance REAL, active BOOLEAN)")
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}

	migDecl, ok := prog.Statements[0].(*compiler.MigrationStmt)
	if !ok {
		t.Fatalf("expected MigrationStmt, got %T", prog.Statements[0])
	}
	if len(migDecl.Tables) != 1 {
		t.Fatalf("expected 1 parsed table, got %d", len(migDecl.Tables))
	}

	table := migDecl.Tables[0]
	if table.Name != "users" {
		t.Errorf("expected table name 'users', got '%s'", table.Name)
	}

	expectedCols := map[string]string{
		"id":      "int",
		"name":    "string",
		"balance": "float64",
		"active":  "bool",
	}

	for _, col := range table.Columns {
		expType, ok := expectedCols[col.Name]
		if !ok {
			t.Errorf("unexpected column: %s", col.Name)
			continue
		}
		if col.Type != expType {
			t.Errorf("column %s: expected type %s, got %s", col.Name, expType, col.Type)
		}
	}
}

func TestORMIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_orm_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
database "sqlite://orm_test.db"

migration "20260616_init" {
	db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, age INTEGER)")
}

extern fn runMigrations() from "go:serv/runtime:RunMigrations"

test "database orm basic execution test" {
	runMigrations()

	// Insert row using ORM
	let user = UsersRow{
		id: 1,
		name: "Alice",
		age: 30
	}
	let err = db.users.insert(user)

	assert err == nil

	// Find row using ORM
	let users, err2 = db.users.find({"id": 1})
	assert err2 == nil
	assert users.length() == 1
	assert users[0].name == "Alice"
	assert users[0].age == 30

	// FindOne
	let singleUser, err3 = db.users.findOne({"id": 1})
	assert err3 == nil
	assert singleUser != nil
	assert singleUser.name == "Alice"

	// Update
	let err4 = db.users.update({"id": 1}, {"age": 31})
	assert err4 == nil

	let updatedUser, err5 = db.users.findOne({"id": 1})
	assert err5 == nil
	assert updatedUser.age == 31

	// Delete
	let err6 = db.users.delete({"id": 1})
	assert err6 == nil

	let deletedUser, err7 = db.users.findOne({"id": 1})
	assert err7 == nil
	assert deletedUser == nil
}

`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	runTests(tmpFile.Name(), false, "")
	
	// Clean up DB
	_ = os.Remove("orm_test.db")
}

