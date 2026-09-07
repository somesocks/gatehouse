package lisp

import (
	"bytes"
	"encoding/xml"
	"io"
	"unicode"
)

const (
	nativeXMLID                = "native:xml/v1"
	nativeXMLMaximumInputBytes = 16 << 20
	nativeXMLMaximumDepth      = 256
	nativeXMLMaximumNodes      = 100000
	nativeXMLMaximumAttributes = 1024
	nativeXMLMaximumTextBytes  = 8 << 20
)

var nativeXMLDecodeDocumentation = doc(
	"(xml/decode text) -> XML Document",
	"Decodes XML text into an xml/document with namespace-aware xml/name values. Comments and XML declarations are ignored, and CDATA is normalized to text.",
	"(xml/decode \"<greeting language='en'>Hello</greeting>\")",
	"(xml/document (xml/element (xml/name \"\" \"greeting\") ((xml/attribute (xml/name \"\" \"language\") \"en\")) ((xml/text \"Hello\"))))",
)

var nativeXMLEncodeDocumentation = doc(
	"(xml/encode document) -> String",
	"Encodes an xml/document as canonical semantic XML. Namespace prefixes and lexical XML details are normalized.",
	"(xml/encode (xml/document (xml/element (xml/name \"\" \"greeting\") (list) (list (xml/text \"Hello\")))))",
	"\"<greeting>Hello</greeting>\"",
)

func nativeXMLModule() Expr {
	decode := withHelp(&builtin{leaky: true, call: pure(nativeXMLDecode)}, nativeXMLDecodeDocumentation.text())
	encode := withHelp(&builtin{leaky: true, call: pure(nativeXMLEncode)}, nativeXMLEncodeDocumentation.text())
	exports := list([]Expr{
		pairValue(symbol("document"), withHelp(&builtin{leaky: true, call: pure(nativeXMLDocument)}, doc("(xml/document root) -> XML Document", "Creates an XML Document with an XML Element root.", "(xml/document (xml/element (xml/name \"\" \"root\") (list) (list)))", "(xml/document (xml/element (xml/name \"\" \"root\") () ()))").text())),
		pairValue(symbol("document?"), withHelp(&builtin{call: pure(nativeXMLDocumentQ)}, doc("(xml/document? value) -> Boolean", "Returns whether value is an XML Document.", "(xml/document? (xml/document (xml/element (xml/name \"\" \"root\") (list) (list))))", "#t").text())),
		pairValue(symbol("document/root"), withHelp(&builtin{leaky: true, call: pure(nativeXMLDocumentRoot)}, doc("(xml/document/root document) -> XML Element", "Returns an XML Document's root element.", "(xml/document/root (xml/document (xml/element (xml/name \"\" \"root\") (list) (list))))", "(xml/element (xml/name \"\" \"root\") () ())").text())),
		pairValue(symbol("element"), withHelp(&builtin{leaky: true, call: pure(nativeXMLElement)}, doc("(xml/element name attributes children) -> XML Element", "Creates an XML Element from an XML Name, a list of XML Attributes, and a list of XML Elements or Text nodes.", "(xml/element (xml/name \"\" \"root\") (list) (list (xml/text \"Hello\")))", "(xml/element (xml/name \"\" \"root\") () ((xml/text \"Hello\")))").text())),
		pairValue(symbol("element?"), withHelp(&builtin{call: pure(nativeXMLElementQ)}, doc("(xml/element? value) -> Boolean", "Returns whether value is an XML Element.", "(xml/element? (xml/element (xml/name \"\" \"root\") (list) (list)))", "#t").text())),
		pairValue(symbol("element/name"), withHelp(&builtin{leaky: true, call: pure(nativeXMLElementName)}, doc("(xml/element/name element) -> XML Name", "Returns an XML Element's name.", "(xml/element/name (xml/element (xml/name \"\" \"root\") (list) (list)))", "(xml/name \"\" \"root\")").text())),
		pairValue(symbol("element/attributes"), withHelp(&builtin{leaky: true, call: pure(nativeXMLElementAttributes)}, doc("(xml/element/attributes element) -> List", "Returns an XML Element's attributes in source order.", "(xml/element/attributes (xml/element (xml/name \"\" \"root\") (list (xml/attribute (xml/name \"\" \"id\") \"1\")) (list)))", "((xml/attribute (xml/name \"\" \"id\") \"1\"))").text())),
		pairValue(symbol("element/children"), withHelp(&builtin{leaky: true, call: pure(nativeXMLElementChildren)}, doc("(xml/element/children element) -> List", "Returns an XML Element's child elements and text nodes in source order.", "(xml/element/children (xml/element (xml/name \"\" \"root\") (list) (list (xml/text \"Hello\"))))", "((xml/text \"Hello\"))").text())),
		pairValue(symbol("name"), withHelp(&builtin{leaky: true, call: pure(nativeXMLName)}, doc("(xml/name namespace local) -> XML Name", "Creates a namespace-aware XML Name from a namespace URI and local name.", "(xml/name \"urn:example\" \"item\")", "(xml/name \"urn:example\" \"item\")").text())),
		pairValue(symbol("name?"), withHelp(&builtin{call: pure(nativeXMLNameQ)}, doc("(xml/name? value) -> Boolean", "Returns whether value is an XML Name.", "(xml/name? (xml/name \"\" \"root\"))", "#t").text())),
		pairValue(symbol("name/namespace"), withHelp(&builtin{leaky: true, call: pure(nativeXMLNameNamespace)}, doc("(xml/name/namespace name) -> String", "Returns an XML Name's namespace URI.", "(xml/name/namespace (xml/name \"urn:example\" \"item\"))", "\"urn:example\"").text())),
		pairValue(symbol("name/local"), withHelp(&builtin{leaky: true, call: pure(nativeXMLNameLocal)}, doc("(xml/name/local name) -> String", "Returns an XML Name's local name.", "(xml/name/local (xml/name \"urn:example\" \"item\"))", "\"item\"").text())),
		pairValue(symbol("attribute"), withHelp(&builtin{leaky: true, call: pure(nativeXMLAttribute)}, doc("(xml/attribute name value) -> XML Attribute", "Creates an XML Attribute from an XML Name and String value.", "(xml/attribute (xml/name \"\" \"id\") \"1\")", "(xml/attribute (xml/name \"\" \"id\") \"1\")").text())),
		pairValue(symbol("attribute?"), withHelp(&builtin{call: pure(nativeXMLAttributeQ)}, doc("(xml/attribute? value) -> Boolean", "Returns whether value is an XML Attribute.", "(xml/attribute? (xml/attribute (xml/name \"\" \"id\") \"1\"))", "#t").text())),
		pairValue(symbol("attribute/name"), withHelp(&builtin{leaky: true, call: pure(nativeXMLAttributeName)}, doc("(xml/attribute/name attribute) -> XML Name", "Returns an XML Attribute's name.", "(xml/attribute/name (xml/attribute (xml/name \"\" \"id\") \"1\"))", "(xml/name \"\" \"id\")").text())),
		pairValue(symbol("attribute/value"), withHelp(&builtin{leaky: true, call: pure(nativeXMLAttributeValue)}, doc("(xml/attribute/value attribute) -> String", "Returns an XML Attribute's value.", "(xml/attribute/value (xml/attribute (xml/name \"\" \"id\") \"1\"))", "\"1\"").text())),
		pairValue(symbol("text"), withHelp(&builtin{leaky: true, call: pure(nativeXMLText)}, doc("(xml/text value) -> XML Text", "Creates an XML Text node.", "(xml/text \"Hello\")", "(xml/text \"Hello\")").text())),
		pairValue(symbol("text?"), withHelp(&builtin{call: pure(nativeXMLTextQ)}, doc("(xml/text? value) -> Boolean", "Returns whether value is XML Text.", "(xml/text? (xml/text \"Hello\"))", "#t").text())),
		pairValue(symbol("text/value"), withHelp(&builtin{leaky: true, call: pure(nativeXMLTextValue)}, doc("(xml/text/value text) -> String", "Returns an XML Text node's value.", "(xml/text/value (xml/text \"Hello\"))", "\"Hello\"").text())),
		pairValue(symbol("element/attributes/get"), withHelp(&builtin{leaky: true, call: pure(nativeXMLElementAttributesGet)}, doc("(xml/element/attributes/get element name) -> String | null", "Returns the first matching attribute value, or null when the attribute is absent.", "(xml/element/attributes/get (xml/element (xml/name \"\" \"item\") (list (xml/attribute (xml/name \"\" \"id\") \"1\")) (list)) (xml/name \"\" \"id\"))", "\"1\"").text())),
		pairValue(symbol("decode"), decode),
		pairValue(symbol("encode"), encode),
	})
	return list([]Expr{symbol("quote"), exports})
}

func nativeXMLDocument(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("xml/document requires one XML Element root"), nil
	}
	if err, _ := nativeXMLElementValues(arguments[0]); err != nil {
		return expressionError("xml/document requires one XML Element root"), nil
	}
	return nil, list([]Expr{symbol("xml/document"), arguments[0]})
}

func nativeXMLElement(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 3 {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	if err, _ := nativeXMLNameValues(arguments[0]); err != nil {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	err, attributes := expressions(arguments[1])
	if err != nil {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	if err := nativeXMLAttributes(arguments[1]); err != nil {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	err, children := expressions(arguments[2])
	if err != nil {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	if err := nativeXMLChildren(arguments[2]); err != nil {
		return expressionError("xml/element requires an XML Name, XML Attribute list, and XML child list"), nil
	}
	return nil, list([]Expr{symbol("xml/element"), arguments[0], list(attributes), list(children)})
}

func nativeXMLName(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("xml/name requires namespace and local Strings"), nil
	}
	if err, _, _ := nativeXMLNameParts(arguments); err != nil {
		return err, nil
	}
	return nil, list([]Expr{symbol("xml/name"), arguments[0], arguments[1]})
}

func nativeXMLAttribute(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("xml/attribute requires an XML Name and String value"), nil
	}
	if err, _ := nativeXMLNameValues(arguments[0]); err != nil {
		return expressionError("xml/attribute requires an XML Name and String value"), nil
	}
	if err, value := requireString(arguments[1]); err != nil || !nativeXMLCharacters(value) {
		return expressionError("xml/attribute requires an XML Name and String value"), nil
	}
	return nil, list([]Expr{symbol("xml/attribute"), arguments[0], arguments[1]})
}

func nativeXMLText(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("xml/text requires one String"), nil
	}
	if err, value := requireString(arguments[0]); err != nil || !nativeXMLCharacters(value) {
		return expressionError("xml/text requires one String"), nil
	}
	return nil, list([]Expr{symbol("xml/text"), arguments[0]})
}

func nativeXMLDocumentQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeXMLTypeQ(arguments, "xml/document", nativeXMLDocumentValues, "xml/document?")
}
func nativeXMLElementQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeXMLTypeQ(arguments, "xml/element", nativeXMLElementValues, "xml/element?")
}
func nativeXMLNameQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeXMLTypeQ(arguments, "xml/name", nativeXMLNameValues, "xml/name?")
}
func nativeXMLAttributeQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeXMLTypeQ(arguments, "xml/attribute", nativeXMLAttributeValues, "xml/attribute?")
}
func nativeXMLTextQ(_ *evaluator, arguments []Expr) (error, Expr) {
	return nativeXMLTypeQ(arguments, "xml/text", nativeXMLTextValues, "xml/text?")
}

func nativeXMLTypeQ(arguments []Expr, tag string, validate func(Expr) (error, []Expr), name string) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("%s requires one argument", name), nil
	}
	err, values := nativeXMLTaggedValues(arguments[0], tag)
	if err != nil {
		return nil, boolean(false)
	}
	if err, _ := validate(arguments[0]); err != nil || len(values) == 0 {
		return nil, boolean(false)
	}
	return nil, boolean(true)
}

func nativeXMLDocumentRoot(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLDocumentValues, "XML Document", "xml/document/root")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[1])
}

func nativeXMLElementName(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLElementValues, "XML Element", "xml/element/name")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[1])
}

func nativeXMLElementAttributes(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLElementValues, "XML Element", "xml/element/attributes")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[2])
}

func nativeXMLElementChildren(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLElementValues, "XML Element", "xml/element/children")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[3])
}

func nativeXMLNameNamespace(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLNameValues, "XML Name", "xml/name/namespace")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[1])
}

func nativeXMLNameLocal(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLNameValues, "XML Name", "xml/name/local")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[2])
}

func nativeXMLAttributeName(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLAttributeValues, "XML Attribute", "xml/attribute/name")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[1])
}

func nativeXMLAttributeValue(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLAttributeValues, "XML Attribute", "xml/attribute/value")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[2])
}

func nativeXMLTextValue(_ *evaluator, arguments []Expr) (error, Expr) {
	err, values := nativeXMLRequire(arguments, nativeXMLTextValues, "XML Text", "xml/text/value")
	if err != nil {
		return err, nil
	}
	return nil, nativeXMLExtract(arguments[0], values[1])
}

func nativeXMLElementAttributesGet(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("xml/element/attributes/get requires an XML Element and XML Name"), nil
	}
	err, values := nativeXMLElementValues(arguments[0])
	if err != nil {
		return expressionError("xml/element/attributes/get requires an XML Element and XML Name"), nil
	}
	err, nameValues := nativeXMLNameValues(arguments[1])
	if err != nil {
		return expressionError("xml/element/attributes/get requires an XML Element and XML Name"), nil
	}
	err, namespace, local := nativeXMLNameParts(nameValues[1:])
	if err != nil {
		return err, nil
	}
	err, attributes := expressions(values[2])
	if err != nil {
		return err, nil
	}
	for _, attribute := range attributes {
		err, attributeValues := nativeXMLAttributeValues(attribute)
		if err != nil {
			return err, nil
		}
		err, attributeNameValues := nativeXMLNameValues(attributeValues[1])
		if err != nil {
			return err, nil
		}
		err, attributeNamespace, attributeLocal := nativeXMLNameParts(attributeNameValues[1:])
		if err != nil {
			return err, nil
		}
		if namespace == attributeNamespace && local == attributeLocal {
			return nil, nativeXMLExtract(arguments[0], attributeValues[2])
		}
	}
	return nil, nativeXMLExtract(arguments[0], null())
}

func nativeXMLDecode(evaluator *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("xml/decode requires one String"), nil
	}
	err, text := requireString(arguments[0])
	if err != nil {
		return err, nil
	}
	if len(text) > nativeXMLMaximumInputBytes {
		return expressionError("xml/decode input exceeds %d bytes", nativeXMLMaximumInputBytes), nil
	}

	decoder := xml.NewDecoder(bytes.NewReader([]byte(text)))
	var root Expr
	stack := make([]nativeXMLDecodeElement, 0)
	nodes, textBytes := 0, 0
	for {
		if evaluator != nil {
			if err := evaluator.interrupted(); err != nil {
				return err, nil
			}
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return expressionError("xml/decode requires valid XML: %v", err), nil
		}
		switch token := token.(type) {
		case xml.StartElement:
			if root != nil && len(stack) == 0 {
				return expressionError("xml/decode requires one XML document"), nil
			}
			if len(stack) >= nativeXMLMaximumDepth {
				return expressionError("xml/decode exceeds maximum depth of %d", nativeXMLMaximumDepth), nil
			}
			if len(token.Attr) > nativeXMLMaximumAttributes {
				return expressionError("xml/decode exceeds maximum attributes of %d", nativeXMLMaximumAttributes), nil
			}
			nodes++
			if nodes > nativeXMLMaximumNodes {
				return expressionError("xml/decode exceeds maximum node count of %d", nativeXMLMaximumNodes), nil
			}
			attributes := make([]Expr, 0, len(token.Attr))
			for _, attribute := range token.Attr {
				if nativeXMLNamespaceDeclaration(attribute.Name) {
					continue
				}
				attributes = append(attributes, nativeXMLAttributeExpr(attribute.Name, attribute.Value))
			}
			stack = append(stack, nativeXMLDecodeElement{name: nativeXMLNameExpr(token.Name), attributes: attributes})
		case xml.EndElement:
			if len(stack) == 0 {
				return expressionError("xml/decode requires valid XML"), nil
			}
			element := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			value := list([]Expr{symbol("xml/element"), element.name, list(element.attributes), list(element.children)})
			if len(stack) == 0 {
				root = value
			} else {
				stack[len(stack)-1].children = append(stack[len(stack)-1].children, value)
			}
		case xml.CharData:
			textBytes += len(token)
			if textBytes > nativeXMLMaximumTextBytes {
				return expressionError("xml/decode exceeds maximum text bytes of %d", nativeXMLMaximumTextBytes), nil
			}
			if len(stack) == 0 {
				if !nativeXMLWhitespace(string(token)) {
					return expressionError("xml/decode requires one XML document"), nil
				}
				continue
			}
			nativeXMLAppendText(&stack[len(stack)-1], string(token), &nodes)
			if nodes > nativeXMLMaximumNodes {
				return expressionError("xml/decode exceeds maximum node count of %d", nativeXMLMaximumNodes), nil
			}
		case xml.Comment:
		case xml.ProcInst:
			if token.Target != "xml" || root != nil || len(stack) != 0 {
				return expressionError("xml/decode does not allow processing instructions"), nil
			}
		case xml.Directive:
			return expressionError("xml/decode does not allow directives or DOCTYPE"), nil
		default:
			return expressionError("xml/decode encountered unsupported XML token"), nil
		}
	}
	if root == nil || len(stack) != 0 {
		return expressionError("xml/decode requires one XML document"), nil
	}
	value := Expr(list([]Expr{symbol("xml/document"), root}))
	if taint := TaintOf(arguments[0]); taint != TaintNone {
		value = withTaint(value, taint)
	}
	return nil, value
}

type nativeXMLDecodeElement struct {
	name                 Expr
	attributes, children []Expr
}

func nativeXMLAppendText(element *nativeXMLDecodeElement, value string, nodes *int) {
	if len(element.children) > 0 {
		last := element.children[len(element.children)-1]
		if err, values := nativeXMLTextValues(last); err == nil {
			_, existing := requireString(values[1])
			element.children[len(element.children)-1] = list([]Expr{symbol("xml/text"), stringValue(existing + value)})
			return
		}
	}
	*nodes++
	element.children = append(element.children, list([]Expr{symbol("xml/text"), stringValue(value)}))
}

func nativeXMLEncode(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("xml/encode requires one XML Document"), nil
	}
	err, values := nativeXMLDocumentValues(arguments[0])
	if err != nil {
		return expressionError("xml/encode requires one XML Document"), nil
	}
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	if err := nativeXMLEncodeElement(encoder, values[1]); err != nil {
		return err, nil
	}
	if err := encoder.Flush(); err != nil {
		return expressionError("xml/encode: %v", err), nil
	}
	return nil, nativeXMLExtract(arguments[0], stringValue(output.String()))
}

func nativeXMLEncodeElement(encoder *xml.Encoder, node Expr) error {
	err, values := nativeXMLElementValues(node)
	if err != nil {
		return expressionError("xml/encode requires XML Element nodes")
	}
	err, nameValues := nativeXMLNameValues(values[1])
	if err != nil {
		return err
	}
	err, namespace, local := nativeXMLNameParts(nameValues[1:])
	if err != nil {
		return err
	}
	err, attributes := expressions(values[2])
	if err != nil {
		return err
	}
	start := xml.StartElement{Name: xml.Name{Space: namespace, Local: local}, Attr: make([]xml.Attr, 0, len(attributes))}
	for _, attribute := range attributes {
		err, attributeValues := nativeXMLAttributeValues(attribute)
		if err != nil {
			return err
		}
		err, attributeNameValues := nativeXMLNameValues(attributeValues[1])
		if err != nil {
			return err
		}
		err, attributeNamespace, attributeLocal := nativeXMLNameParts(attributeNameValues[1:])
		if err != nil {
			return err
		}
		err, value := requireString(attributeValues[2])
		if err != nil {
			return err
		}
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Space: attributeNamespace, Local: attributeLocal}, Value: value})
	}
	if err := encoder.EncodeToken(start); err != nil {
		return expressionError("xml/encode: %v", err)
	}
	err, children := expressions(values[3])
	if err != nil {
		return err
	}
	for _, child := range children {
		err, childValues := nativeXMLTaggedValues(child, "xml/text")
		if err == nil {
			err, value := requireString(childValues[1])
			if err != nil {
				return err
			}
			if err := encoder.EncodeToken(xml.CharData(value)); err != nil {
				return expressionError("xml/encode: %v", err)
			}
			continue
		}
		if err := nativeXMLEncodeElement(encoder, child); err != nil {
			return err
		}
	}
	if err := encoder.EncodeToken(xml.EndElement{Name: start.Name}); err != nil {
		return expressionError("xml/encode: %v", err)
	}
	return nil
}

func nativeXMLDocumentValues(node Expr) (error, []Expr) {
	err, values := nativeXMLTaggedValues(node, "xml/document")
	if err != nil || len(values) != 2 {
		return expressionError("requires an XML Document"), nil
	}
	if err, _ := nativeXMLElementValues(values[1]); err != nil {
		return err, nil
	}
	return nil, values
}

func nativeXMLElementValues(node Expr) (error, []Expr) {
	err, values := nativeXMLTaggedValues(node, "xml/element")
	if err != nil || len(values) != 4 {
		return expressionError("requires an XML Element"), nil
	}
	if err, _ := nativeXMLNameValues(values[1]); err != nil {
		return err, nil
	}
	if err := nativeXMLAttributes(values[2]); err != nil {
		return err, nil
	}
	if err := nativeXMLChildren(values[3]); err != nil {
		return err, nil
	}
	return nil, values
}

func nativeXMLNameValues(node Expr) (error, []Expr) {
	err, values := nativeXMLTaggedValues(node, "xml/name")
	if err != nil || len(values) != 3 {
		return expressionError("requires an XML Name"), nil
	}
	if err, _, _ := nativeXMLNameParts(values[1:]); err != nil {
		return err, nil
	}
	return nil, values
}

func nativeXMLNameParts(values []Expr) (error, string, string) {
	namespaceErr, namespace := requireString(values[0])
	localErr, local := requireString(values[1])
	if namespaceErr != nil || localErr != nil || !nativeXMLCharacters(namespace) || !nativeXMLLocalName(local) {
		return expressionError("xml/name requires namespace and local Strings"), "", ""
	}
	return nil, namespace, local
}

func nativeXMLAttributeValues(node Expr) (error, []Expr) {
	err, values := nativeXMLTaggedValues(node, "xml/attribute")
	if err != nil || len(values) != 3 {
		return expressionError("requires an XML Attribute"), nil
	}
	if err, _ := nativeXMLNameValues(values[1]); err != nil {
		return err, nil
	}
	if err, value := requireString(values[2]); err != nil || !nativeXMLCharacters(value) {
		return expressionError("requires an XML Attribute"), nil
	}
	return nil, values
}

func nativeXMLTextValues(node Expr) (error, []Expr) {
	err, values := nativeXMLTaggedValues(node, "xml/text")
	if err != nil || len(values) != 2 {
		return expressionError("requires XML Text"), nil
	}
	if err, value := requireString(values[1]); err != nil || !nativeXMLCharacters(value) {
		return expressionError("requires XML Text"), nil
	}
	return nil, values
}

func nativeXMLAttributes(value Expr) error {
	err, attributes := expressions(value)
	if err != nil {
		return expressionError("requires an XML Attribute list")
	}
	seen := make(map[string]struct{}, len(attributes))
	for _, attribute := range attributes {
		err, values := nativeXMLAttributeValues(attribute)
		if err != nil {
			return err
		}
		err, nameValues := nativeXMLNameValues(values[1])
		if err != nil {
			return err
		}
		err, namespace, local := nativeXMLNameParts(nameValues[1:])
		if err != nil || nativeXMLNamespaceDeclaration(xml.Name{Space: namespace, Local: local}) {
			return expressionError("requires an XML Attribute list")
		}
		key := namespace + "\x00" + local
		if _, exists := seen[key]; exists {
			return expressionError("requires an XML Attribute list")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func nativeXMLChildren(value Expr) error {
	err, children := expressions(value)
	if err != nil {
		return expressionError("requires an XML child list")
	}
	for _, child := range children {
		if err, _ := nativeXMLTextValues(child); err == nil {
			continue
		}
		if err, _ := nativeXMLElementValues(child); err != nil {
			return expressionError("requires XML Element or Text children")
		}
	}
	return nil
}

func nativeXMLTaggedValues(node Expr, tag string) (error, []Expr) {
	err, values := expressions(node)
	if err != nil || len(values) == 0 || !isSymbol(values[0], tag) {
		return expressionError("requires %s", tag), nil
	}
	return nil, values
}

func nativeXMLRequire(arguments []Expr, validate func(Expr) (error, []Expr), typeName string, name string) (error, []Expr) {
	if len(arguments) != 1 {
		return expressionError("%s requires one %s", name, typeName), nil
	}
	err, values := validate(arguments[0])
	if err != nil {
		return expressionError("%s requires an %s", name, typeName), nil
	}
	return nil, values
}

func nativeXMLNameExpr(name xml.Name) Expr {
	return list([]Expr{symbol("xml/name"), stringValue(name.Space), stringValue(name.Local)})
}
func nativeXMLAttributeExpr(name xml.Name, value string) Expr {
	return list([]Expr{symbol("xml/attribute"), nativeXMLNameExpr(name), stringValue(value)})
}

func nativeXMLNamespaceDeclaration(name xml.Name) bool {
	return name.Space == "xmlns" || (name.Space == "" && name.Local == "xmlns")
}

func nativeXMLExtract(source Expr, value Expr) Expr {
	if taint := TaintOf(source); taint != TaintNone {
		return withTaint(value, taint)
	}
	return value
}

func nativeXMLWhitespace(value string) bool {
	for _, character := range value {
		if character != ' ' && character != '\t' && character != '\r' && character != '\n' {
			return false
		}
	}
	return true
}

func nativeXMLCharacters(value string) bool {
	for _, character := range value {
		if character != '\t' && character != '\n' && character != '\r' && (character < 0x20 || (character >= 0xD800 && character <= 0xDFFF) || character == 0xFFFE || character == 0xFFFF || character > 0x10FFFF) {
			return false
		}
	}
	return true
}

func nativeXMLLocalName(value string) bool {
	for index, character := range []rune(value) {
		if index == 0 {
			if character != '_' && !unicode.IsLetter(character) {
				return false
			}
			continue
		}
		if character != '_' && character != '-' && character != '.' && !unicode.IsLetter(character) && !unicode.IsDigit(character) && !unicode.IsMark(character) {
			return false
		}
	}
	return value != ""
}
