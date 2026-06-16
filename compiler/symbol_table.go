package compiler

// Symbol represents a declared identifier (variable, parameter, function, struct, etc.)
// inside the compiler's symbol table.
type Symbol struct {
	Name  string
	Type  string // "variable", "parameter", "function", "struct", etc.
	Token Token  // Token of the declaration for diagnostic line/col info
	Used  bool   // Track if the symbol was referenced
}

// Scope represents a lexical block scope in the program.
// Scopes form a parent-pointer tree structure.
type Scope struct {
	Parent   *Scope
	Symbols  map[string]*Symbol
	Children []*Scope
}

// NewScope creates a new child scope under the given parent scope.
func NewScope(parent *Scope) *Scope {
	s := &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
	if parent != nil {
		parent.Children = append(parent.Children, s)
	}
	return s
}

// Insert adds a symbol to the current scope level.
// Returns false if the symbol already exists in this specific scope level.
func (s *Scope) Insert(sym *Symbol) bool {
	if _, exists := s.Symbols[sym.Name]; exists {
		return false
	}
	s.Symbols[sym.Name] = sym
	return true
}

// Lookup searches for a symbol by name, walking up the parent scope chain if needed.
// Returns the symbol and true if found, nil and false otherwise.
func (s *Scope) Lookup(name string) (*Symbol, bool) {
	curr := s
	for curr != nil {
		if sym, exists := curr.Symbols[name]; exists {
			return sym, true
		}
		curr = curr.Parent
	}
	return nil, false
}

// LookupLocal searches for a symbol only in the current scope level.
func (s *Scope) LookupLocal(name string) (*Symbol, bool) {
	sym, exists := s.Symbols[name]
	return sym, exists
}

// declareVar inserts a variable symbol into the current scope level of Codegen.
func (c *Codegen) declareVar(name string) {
	c.declaredVars.Insert(&Symbol{Name: name, Type: "variable"})
}

// isDeclared checks if a variable name is declared in the current lexical scope chain of Codegen.
func (c *Codegen) isDeclared(name string) bool {
	_, found := c.declaredVars.Lookup(name)
	return found
}
