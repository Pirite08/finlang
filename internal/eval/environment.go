package eval

import (
	"fmt"

	"github.com/Pirite08/finlang/internal/diagnostic"
)

type Environment struct {
	values  map[string]Value
	mutable map[string]struct{}
	parent  *Environment
}

func NewEnvironment(parent *Environment) *Environment {
	return &Environment{values: make(map[string]Value), parent: parent}
}

func (e *Environment) Define(name string, value Value) error {
	return e.define(name, value, false)
}

func (e *Environment) define(name string, value Value, mutable bool) error {
	if _, exists := e.values[name]; exists {
		return diagnostic.New("E_DUPLICATE_BINDING", fmt.Sprintf("name %q is already defined in this scope", name))
	}
	e.values[name] = value
	if mutable {
		if e.mutable == nil {
			e.mutable = make(map[string]struct{})
		}
		e.mutable[name] = struct{}{}
	}
	return nil
}

func (e *Environment) Lookup(name string) (Value, bool) {
	value, scope := e.resolve(name)
	return value, scope != nil
}

func (e *Environment) resolve(name string) (Value, *Environment) {
	for current := e; current != nil; current = current.parent {
		if value, ok := current.values[name]; ok {
			return value, current
		}
	}
	return nil, nil
}

func (e *Environment) assignmentScope(name string) (*Environment, error) {
	_, scope := e.resolve(name)
	if scope == nil {
		return nil, diagnostic.New("E_UNDEFINED_NAME", fmt.Sprintf("name %q is not defined", name))
	}
	if _, ok := scope.mutable[name]; !ok {
		return nil, diagnostic.New("E_IMMUTABLE_BINDING", fmt.Sprintf("name %q cannot be assigned; declare a mutable variable with var", name))
	}
	return scope, nil
}
