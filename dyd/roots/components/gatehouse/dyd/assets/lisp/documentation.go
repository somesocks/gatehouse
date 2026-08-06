package lisp

type documentation struct {
	signature   string
	description string
	example     string
	result      string
}

func doc(signature string, description string, example string, result string) documentation {
	return documentation{
		signature:   signature,
		description: description,
		example:     example,
		result:      result,
	}
}

func (documentation documentation) text() string {
	return documentation.signature + "\n" + documentation.description + "\nExample: " + documentation.example + " => " + documentation.result + "."
}
