// Package inputform defines the durable, versioned form descriptor and its
// server-side result validation. Lisp constructors are converted to this form
// before an input request is stored.
package inputform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

const Version = 1

type Form struct {
	Version int     `json:"version"`
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Fields  []Field `json:"fields"`
}

type Field struct {
	ID        string   `json:"id,omitempty"`
	Label     string   `json:"label,omitempty"`
	Type      string   `json:"type"`
	Optional  bool     `json:"optional,omitempty"`
	MinLength *int64   `json:"min_length,omitempty"`
	MaxLength *int64   `json:"max_length,omitempty"`
	Min       *int64   `json:"min,omitempty"`
	Max       *int64   `json:"max,omitempty"`
	Integer   bool     `json:"integer,omitempty"`
	Choices   []string `json:"choices,omitempty"`
	Fields    []Field  `json:"fields,omitempty"`
	Item      *Field   `json:"item,omitempty"`
}

// Decode rejects unknown descriptor properties and malformed or unsupported
// fields, including descriptors fabricated without the Lisp constructors.
func Decode(source []byte) (error, *Form) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	var form Form
	if err := decoder.Decode(&form); err != nil {
		return fmt.Errorf("decode input form: %w", err), nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode input form: expected one object"), nil
	}
	if err := form.Validate(); err != nil {
		return err, nil
	}
	return nil, &form
}

func (form Form) Validate() error {
	if form.Version != Version || form.Type != "form" || strings.TrimSpace(form.Title) == "" {
		return fmt.Errorf("input form requires a supported version, type, and title")
	}
	return validateFields(form.Fields)
}

func validateFields(fields []Field) error {
	if len(fields) == 0 {
		return fmt.Errorf("input form requires fields")
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if err := validateField(field, true); err != nil {
			return err
		}
		if seen[field.ID] {
			return fmt.Errorf("input form field %q is duplicated", field.ID)
		}
		seen[field.ID] = true
	}
	return nil
}

func validateField(field Field, named bool) error {
	if named {
		if strings.TrimSpace(field.ID) == "" || strings.TrimSpace(field.Label) == "" {
			return fmt.Errorf("input form field requires an id and label")
		}
	} else if field.ID != "" || field.Label != "" || field.Optional {
		return fmt.Errorf("input form list item must not have an id, label, or optional wrapper")
	}
	switch field.Type {
	case "object":
		if field.Item != nil || len(field.Choices) != 0 || hasConstraints(field) {
			return fmt.Errorf("input form object has unsupported properties")
		}
		return validateFields(field.Fields)
	case "list":
		if field.Item == nil || len(field.Fields) != 0 || len(field.Choices) != 0 || hasConstraints(field) {
			return fmt.Errorf("input form list requires one item type")
		}
		return validateField(*field.Item, false)
	case "text":
		if field.Item != nil || len(field.Fields) != 0 || len(field.Choices) != 0 || field.Min != nil || field.Max != nil || field.Integer {
			return fmt.Errorf("input form text has unsupported properties")
		}
		if field.MinLength != nil && *field.MinLength < 0 || field.MaxLength != nil && *field.MaxLength < 0 || field.MinLength != nil && field.MaxLength != nil && *field.MinLength > *field.MaxLength {
			return fmt.Errorf("input form text length bounds are invalid")
		}
	case "number":
		if field.Item != nil || len(field.Fields) != 0 || len(field.Choices) != 0 || field.MinLength != nil || field.MaxLength != nil {
			return fmt.Errorf("input form number has unsupported properties")
		}
		if field.Min != nil && field.Max != nil && *field.Min > *field.Max {
			return fmt.Errorf("input form number bounds are invalid")
		}
	case "boolean":
		if field.Item != nil || len(field.Fields) != 0 || len(field.Choices) != 0 || hasConstraints(field) {
			return fmt.Errorf("input form boolean has unsupported properties")
		}
	case "options":
		if field.Item != nil || len(field.Fields) != 0 || hasConstraints(field) || len(field.Choices) == 0 {
			return fmt.Errorf("input form options require choices")
		}
		seen := make(map[string]bool, len(field.Choices))
		for _, choice := range field.Choices {
			if strings.TrimSpace(choice) == "" || seen[choice] {
				return fmt.Errorf("input form options require distinct non-blank choices")
			}
			seen[choice] = true
		}
	default:
		return fmt.Errorf("input form field type %q is unsupported", field.Type)
	}
	return nil
}

func hasConstraints(field Field) bool {
	return field.MinLength != nil || field.MaxLength != nil || field.Min != nil || field.Max != nil || field.Integer
}

// ValidateResult checks the complete result object. An absent optional key is
// distinct from JSON null, and unknown keys are not accepted.
func (form Form) ValidateResult(source json.RawMessage) error {
	if err := form.Validate(); err != nil {
		return err
	}
	return validateObject(form.Fields, source)
}

// FieldAtPath resolves a named field through objects. Lists must be replaced
// as a whole rather than patched by array index.
func (form Form) FieldAtPath(path []string) (Field, bool) {
	if len(path) == 0 {
		return Field{}, false
	}
	fields := form.Fields
	for index, segment := range path {
		found := false
		for _, field := range fields {
			if field.ID != segment {
				continue
			}
			if index == len(path)-1 {
				return field, true
			}
			if field.Type != "object" {
				return Field{}, false
			}
			fields = field.Fields
			found = true
			break
		}
		if !found {
			return Field{}, false
		}
	}
	return Field{}, false
}

// ValidateDraftValue checks every supplied value without requiring fields
// omitted from an unfinished object or list entry. ValidateResult performs
// the complete required-field check on Submit.
func ValidateDraftValue(field Field, source json.RawMessage) error {
	switch field.Type {
	case "object":
		var object map[string]json.RawMessage
		if err := json.Unmarshal(source, &object); err != nil || object == nil {
			return fmt.Errorf("requires a JSON object")
		}
		allowed := make(map[string]Field, len(field.Fields))
		for _, child := range field.Fields {
			allowed[child.ID] = child
		}
		for key, value := range object {
			child, ok := allowed[key]
			if !ok {
				return fmt.Errorf("input form field %q is unknown", key)
			}
			if err := ValidateDraftValue(child, value); err != nil {
				return fmt.Errorf("input form field %q: %w", key, err)
			}
		}
		return nil
	case "list":
		var items []json.RawMessage
		if err := json.Unmarshal(source, &items); err != nil || items == nil {
			return fmt.Errorf("requires a JSON array")
		}
		for _, item := range items {
			if err := ValidateDraftValue(*field.Item, item); err != nil {
				return fmt.Errorf("invalid list item: %w", err)
			}
		}
		return nil
	default:
		return validateValue(field, source)
	}
}

func validateObject(fields []Field, source json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(source, &object); err != nil || object == nil {
		return fmt.Errorf("input form result requires a JSON object")
	}
	allowed := make(map[string]Field, len(fields))
	for _, field := range fields {
		allowed[field.ID] = field
		value, present := object[field.ID]
		if !present {
			if !field.Optional {
				return fmt.Errorf("input form field %q is required", field.ID)
			}
			continue
		}
		if err := validateValue(field, value); err != nil {
			return fmt.Errorf("input form field %q: %w", field.ID, err)
		}
	}
	for key := range object {
		if _, exists := allowed[key]; !exists {
			return fmt.Errorf("input form field %q is unknown", key)
		}
	}
	return nil
}

func validateValue(field Field, source json.RawMessage) error {
	switch field.Type {
	case "object":
		return validateObject(field.Fields, source)
	case "list":
		var items []json.RawMessage
		if err := json.Unmarshal(source, &items); err != nil || items == nil {
			return fmt.Errorf("requires a JSON array")
		}
		for _, item := range items {
			if err := validateValue(*field.Item, item); err != nil {
				return fmt.Errorf("invalid list item: %w", err)
			}
		}
	case "text", "options":
		var text string
		if len(source) == 0 || source[0] != '"' || json.Unmarshal(source, &text) != nil {
			return fmt.Errorf("requires a JSON string")
		}
		if field.Type == "options" {
			for _, choice := range field.Choices {
				if text == choice {
					return nil
				}
			}
			return fmt.Errorf("value is not a configured choice")
		}
		length := int64(utf8.RuneCountInString(text))
		if field.MinLength != nil && length < *field.MinLength || field.MaxLength != nil && length > *field.MaxLength {
			return fmt.Errorf("text length is out of bounds")
		}
	case "boolean":
		var answer bool
		if !bytes.Equal(source, []byte("true")) && !bytes.Equal(source, []byte("false")) || json.Unmarshal(source, &answer) != nil {
			return fmt.Errorf("requires a JSON boolean")
		}
	case "number":
		decoder := json.NewDecoder(bytes.NewReader(source))
		decoder.UseNumber()
		var raw any
		if err := decoder.Decode(&raw); err != nil {
			return fmt.Errorf("requires a JSON number")
		}
		number, ok := raw.(json.Number)
		if !ok {
			return fmt.Errorf("requires a JSON number")
		}
		// An otherwise tiny JSON value such as 1e100000000 must not cause
		// unbounded allocation when compared exactly with integer bounds.
		if len(number) > 4096 {
			return fmt.Errorf("requires a supported JSON number")
		}
		if at := strings.IndexAny(string(number), "eE"); at >= 0 {
			exponent, err := strconv.ParseInt(string(number)[at+1:], 10, 32)
			if err != nil || exponent < -10000 || exponent > 10000 {
				return fmt.Errorf("requires a supported JSON number")
			}
		}
		value, ok := new(big.Rat).SetString(string(number))
		if !ok {
			return fmt.Errorf("requires a supported JSON number")
		}
		if field.Integer && !value.IsInt() || field.Min != nil && value.Cmp(new(big.Rat).SetInt64(*field.Min)) < 0 || field.Max != nil && value.Cmp(new(big.Rat).SetInt64(*field.Max)) > 0 {
			return fmt.Errorf("number is out of bounds")
		}
	}
	return nil
}
