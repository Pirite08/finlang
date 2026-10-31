package eval

import (
	"example.com/finlang/internal/diagnostic"
	"fmt"
)

type Environment struct {
	values map[string]Value
	parent *Environment
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{values: make(map[string]Value), parent: parent}
}

func (e *Environment) Define(name string, value Value) error {
	if _, exists := e.values[name]; exists {
		return diagnostic.New("E_DUPLICATE_BINDING", fmt.Sprintf("name %q is already defined in this scope", name))
	}
	e.values[name] = value
	return nil
}

func (e *Environment) Lookup(name string) (Value, bool) {
	for current := e; current != nil; current = current.parent {
		if value, ok := current.values[name]; ok {
			return value, true
		}
	}
	return nil, false
}
