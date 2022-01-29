package backend

import "strings"

type (
	// InputError collection.
	InputErrors []InputError

	// InputError contains field name and error type.
	InputError struct {
		FullName string
		Name     string
		Kind     string
		Type     string
		Param    string
	}

	// InputErrorWithIndex contains InputError and struct index in a slice.
	InputErrorWithIndex struct {
		InputError
		Index int
	}
)

func (err InputError) Error() string {
	return err.Name + ": " + err.Type
}

func (errs InputErrors) Error() string {
	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return "Errors: " + strings.Join(msgs, ", ")
}

func (errs InputErrors) PanicIfPresent() {
	if len(errs) > 0 {
		panic(errs)
	}
}

func NewInputErrors(name, errType string) InputErrors {
	return InputErrors{NewInputError(name, errType)}
}

func NewInputError(name, errType string) InputError {
	return InputError{
		FullName: name,
		Name:     name,
		Kind:     "string",
		Type:     errType,
		Param:    "",
	}
}
