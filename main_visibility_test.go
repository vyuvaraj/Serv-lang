package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisibilityEnforcement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "serv_visibility_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create helper module with exported and non-exported symbols
	helperContent := `
export struct User {
	name: string
}

export fn createUser(name: string) -> User {
	return User { name: name }
}

export fn User.exportedMethod() -> string {
	return "public"
}

fn User.privateMethod() -> string {
	return "private"
}

fn privateHelper() -> string {
	return "secret"
}

route "GET" "/helper-route" (req) {
	return { "ok": true }
}
`
	helperPath := filepath.Join(tmpDir, "helper.srv")
	if err := os.WriteFile(helperPath, []byte(helperContent), 0644); err != nil {
		t.Fatalf("failed to write helper.srv: %v", err)
	}

	// Case 1: Valid selective import (User and createUser)
	main1Content := `
import { User, createUser } from "./helper.srv"
`
	main1Path := filepath.Join(tmpDir, "main1.srv")
	if err := os.WriteFile(main1Path, []byte(main1Content), 0644); err != nil {
		t.Fatalf("failed to write main1.srv: %v", err)
	}

	prog1, err := parseWithDependencies(main1Path, make(map[string]int))
	if err != nil {
		t.Errorf("expected no error for valid selective import, got: %v", err)
	}
	if prog1 == nil || len(prog1.Statements) == 0 {
		t.Errorf("expected imported statements in program")
	}

	// Check that privateHelper is NOT imported, but User and createUser are
	hasUser := false
	hasCreateUser := false
	hasPrivateHelper := false
	hasHelperRoute := false
	hasExportedMethod := false
	hasPrivateMethod := false

	for _, stmt := range prog1.Statements {
		name := statementName(stmt)
		switch name {
		case "User":
			hasUser = true
		case "createUser":
			hasCreateUser = true
		case "privateHelper":
			hasPrivateHelper = true
		case "User.exportedMethod":
			hasExportedMethod = true
		case "User.privateMethod":
			hasPrivateMethod = true
		}
		if strings.Contains(stmt.String(), "/helper-route") {
			hasHelperRoute = true
		}
	}

	if !hasUser || !hasCreateUser {
		t.Errorf("expected User and createUser to be imported")
	}
	if hasPrivateHelper {
		t.Errorf("expected privateHelper to NOT be imported in selective import")
	}
	if !hasHelperRoute {
		t.Errorf("expected non-named route to be imported automatically")
	}
	if !hasExportedMethod {
		t.Errorf("expected exported method User.exportedMethod to be imported automatically")
	}
	if hasPrivateMethod {
		t.Errorf("expected private method User.privateMethod to NOT be imported")
	}

	// Case 2: Invalid selective import of non-exported symbol (privateHelper)
	main2Content := `
import { User, privateHelper } from "./helper.srv"
`
	main2Path := filepath.Join(tmpDir, "main2.srv")
	if err := os.WriteFile(main2Path, []byte(main2Content), 0644); err != nil {
		t.Fatalf("failed to write main2.srv: %v", err)
	}

	_, err = parseWithDependencies(main2Path, make(map[string]int))
	if err == nil {
		t.Errorf("expected error when importing non-exported symbol 'privateHelper', got nil")
	} else if !strings.Contains(err.Error(), "cannot import non-exported symbol 'privateHelper'") {
		t.Errorf("expected error message about non-exported symbol, got: %v", err)
	}

	// Case 3: Invalid selective import of non-existent symbol (fooBar)
	main3Content := `
import { User, fooBar } from "./helper.srv"
`
	main3Path := filepath.Join(tmpDir, "main3.srv")
	if err := os.WriteFile(main3Path, []byte(main3Content), 0644); err != nil {
		t.Fatalf("failed to write main3.srv: %v", err)
	}

	_, err = parseWithDependencies(main3Path, make(map[string]int))
	if err == nil {
		t.Errorf("expected error when importing non-existent symbol 'fooBar', got nil")
	} else if !strings.Contains(err.Error(), "symbol 'fooBar' is not defined") {
		t.Errorf("expected error message about undefined symbol, got: %v", err)
	}

	// Case 4: Wildcard import (should ONLY import exported named symbols and all non-named statements)
	main4Content := `
import "./helper.srv"
`
	main4Path := filepath.Join(tmpDir, "main4.srv")
	if err := os.WriteFile(main4Path, []byte(main4Content), 0644); err != nil {
		t.Fatalf("failed to write main4.srv: %v", err)
	}

	prog4, err := parseWithDependencies(main4Path, make(map[string]int))
	if err != nil {
		t.Errorf("expected no error for wildcard import, got: %v", err)
	}

	hasUser = false
	hasCreateUser = false
	hasPrivateHelper = false
	hasHelperRoute = false
	hasExportedMethod = false
	hasPrivateMethod = false

	for _, stmt := range prog4.Statements {
		name := statementName(stmt)
		switch name {
		case "User":
			hasUser = true
		case "createUser":
			hasCreateUser = true
		case "privateHelper":
			hasPrivateHelper = true
		case "User.exportedMethod":
			hasExportedMethod = true
		case "User.privateMethod":
			hasPrivateMethod = true
		}
		if strings.Contains(stmt.String(), "/helper-route") {
			hasHelperRoute = true
		}
	}

	if !hasUser || !hasCreateUser {
		t.Errorf("wildcard import: expected User and createUser to be imported")
	}
	if hasPrivateHelper {
		t.Errorf("wildcard import: expected privateHelper to NOT be imported")
	}
	if !hasHelperRoute {
		t.Errorf("wildcard import: expected non-named route to be imported automatically")
	}
	if !hasExportedMethod {
		t.Errorf("wildcard import: expected exported method User.exportedMethod to be imported")
	}
	if hasPrivateMethod {
		t.Errorf("wildcard import: expected private method User.privateMethod to NOT be imported")
	}
}


