package agent

import (
	"fmt"
	"strings"

	"gatehouse/inputform"
	"gatehouse/lisp"
)

// inputFormDescriptor converts a validated tagged Lisp form into the immutable
// versioned descriptor stored on input.request.
func inputFormDescriptor(value lisp.Expr) (error, inputform.Form) {
	if err := validateInputForm(value); err != nil {
		return err, inputform.Form{}
	}
	_, values := lisp.RequireList(value)
	_, title := lisp.RequireString(values[1])
	form := inputform.Form{Version: inputform.Version, Type: "form", Title: title}
	for _, child := range values[2:] {
		err, field := inputFieldDescriptor(child)
		if err != nil {
			return err, inputform.Form{}
		}
		form.Fields = append(form.Fields, field)
	}
	if err := form.Validate(); err != nil {
		return err, inputform.Form{}
	}
	return nil, form
}

func inputFieldDescriptor(value lisp.Expr) (error, inputform.Field) {
	err, values := lisp.RequireList(value)
	if err != nil || len(values) == 0 {
		return fmt.Errorf("input field descriptor is invalid"), inputform.Field{}
	}
	err, tag := lisp.RequireSymbol(values[0])
	if err != nil {
		return err, inputform.Field{}
	}
	if tag == "input/form/optional" {
		err, field := inputFieldDescriptor(values[1])
		field.Optional = true
		return err, field
	}
	if tag == "input/form/list" {
		err, field := inputFieldDescriptor(values[1])
		if err != nil {
			return err, inputform.Field{}
		}
		item := field
		item.ID, item.Label = "", ""
		return nil, inputform.Field{ID: field.ID, Label: field.Label, Type: "list", Item: &item}
	}
	if len(values) < 3 {
		return fmt.Errorf("input field descriptor is invalid"), inputform.Field{}
	}
	_, id := lisp.RequireString(values[1])
	_, label := lisp.RequireString(values[2])
	field := inputform.Field{ID: id, Label: label, Type: strings.TrimPrefix(tag, "input/form/")}
	switch tag {
	case "input/form/custom":
		err, custom := inputCustomDefinition(values[1:])
		if err != nil {
			return err, inputform.Field{}
		}
		field.Custom = custom
	case "input/form/object":
		for _, child := range values[3:] {
			err, nested := inputFieldDescriptor(child)
			if err != nil {
				return err, inputform.Field{}
			}
			field.Fields = append(field.Fields, nested)
		}
	case "input/form/options":
		_, choices := lisp.RequireList(values[3])
		for _, choice := range choices {
			_, text := lisp.RequireString(choice)
			field.Choices = append(field.Choices, text)
		}
	case "input/form/text", "input/form/number":
		for _, constraint := range values[3:] {
			_, parts := lisp.RequireList(constraint)
			_, name := lisp.RequireSymbol(parts[0])
			if name == "input/form/number/integer" {
				field.Integer = true
				continue
			}
			_, bound := lisp.RequireInteger(parts[1])
			switch name {
			case "input/form/text/min-length":
				field.MinLength = &bound
			case "input/form/text/max-length":
				field.MaxLength = &bound
			case "input/form/number/min":
				field.Min = &bound
			case "input/form/number/max":
				field.Max = &bound
			}
		}
	case "input/form/boolean":
	case "input/form/files":
		for _, constraint := range values[3:] {
			_, parts := lisp.RequireList(constraint)
			_, name := lisp.RequireSymbol(parts[0])
			switch name {
			case "input/form/files/max-files":
				_, bound := lisp.RequireInteger(parts[1])
				field.MaxFiles = &bound
			case "input/form/files/media-types":
				_, types := lisp.RequireList(parts[1])
				for _, candidate := range types {
					_, mediaType := lisp.RequireString(candidate)
					field.MediaTypes = append(field.MediaTypes, mediaType)
				}
			}
		}
	default:
		return fmt.Errorf("input field descriptor type %q is unsupported", tag), inputform.Field{}
	}
	return nil, field
}
