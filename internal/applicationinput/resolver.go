package applicationinput

import (
	"fmt"
	"strings"
)

type Class string

const (
	ClassDefault   Class = "default"
	ClassGenerated Class = "generated"
	ClassExternal  Class = "external"
)

type Source string

const (
	SourceSupplied  Source = "supplied"
	SourceDefault   Source = "default"
	SourceGenerated Source = "generated"
)

type Condition struct {
	Input  string
	Equals string
}

type Definition struct {
	Name       string
	Label      string
	Class      Class
	Default    string
	Secret     bool
	Required   bool
	RequiredIf *Condition
}

type Value struct {
	Name   string
	Value  string
	Secret bool
	Source Source
}

func (v Value) String() string {
	if v.Secret {
		return "<redacted>"
	}
	return v.Value
}

type Result struct {
	Values     map[string]Value
	Unresolved []Definition
}

type Generator func(Definition) (string, error)

func Resolve(definitions []Definition, supplied map[string]string, generate Generator) (Result, error) {
	result := Result{Values: map[string]Value{}}
	context := map[string]string{}
	for key, value := range supplied {
		context[key] = strings.TrimSpace(value)
	}

	seen := map[string]struct{}{}
	for _, definition := range definitions {
		definition.Name = strings.TrimSpace(definition.Name)
		if definition.Name == "" {
			return Result{}, fmt.Errorf("application input name is required")
		}
		if _, exists := seen[definition.Name]; exists {
			return Result{}, fmt.Errorf("duplicate application input %q", definition.Name)
		}
		seen[definition.Name] = struct{}{}
		if err := validateClass(definition.Class); err != nil {
			return Result{}, fmt.Errorf("application input %s: %w", definition.Name, err)
		}

		required := definition.Required || conditionMatches(definition.RequiredIf, context)
		if value := strings.TrimSpace(supplied[definition.Name]); value != "" {
			result.Values[definition.Name] = Value{Name: definition.Name, Value: value, Secret: definition.Secret, Source: SourceSupplied}
			context[definition.Name] = value
			continue
		}

		switch definition.Class {
		case ClassDefault:
			value := strings.TrimSpace(definition.Default)
			if value != "" {
				result.Values[definition.Name] = Value{Name: definition.Name, Value: value, Secret: definition.Secret, Source: SourceDefault}
				context[definition.Name] = value
				continue
			}
		case ClassGenerated:
			if generate != nil && required {
				value, err := generate(definition)
				if err != nil {
					return Result{}, fmt.Errorf("generate application input %s: %w", definition.Name, err)
				}
				value = strings.TrimSpace(value)
				if value != "" {
					result.Values[definition.Name] = Value{Name: definition.Name, Value: value, Secret: definition.Secret, Source: SourceGenerated}
					context[definition.Name] = value
					continue
				}
			}
		}

		if required {
			result.Unresolved = append(result.Unresolved, definition)
		}
	}
	return result, nil
}

func PersistableValues(result Result) map[string]string {
	values := map[string]string{}
	for name, value := range result.Values {
		if value.Secret {
			continue
		}
		values[name] = value.Value
	}
	return values
}

func SecretValues(result Result) map[string]string {
	values := map[string]string{}
	for name, value := range result.Values {
		if !value.Secret {
			continue
		}
		values[name] = value.Value
	}
	return values
}

func conditionMatches(condition *Condition, values map[string]string) bool {
	if condition == nil {
		return false
	}
	return strings.TrimSpace(values[condition.Input]) == strings.TrimSpace(condition.Equals)
}

func validateClass(class Class) error {
	switch class {
	case ClassDefault, ClassGenerated, ClassExternal:
		return nil
	default:
		return fmt.Errorf("unsupported input class %q", class)
	}
}
