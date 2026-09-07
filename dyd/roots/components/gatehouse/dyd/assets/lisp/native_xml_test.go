package lisp

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNativeXML(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{`(import (xml @native:xml/v1) (xml/document (xml/element (xml/name "" "root") (list) (list))))`, `(xml/document (xml/element (xml/name "" "root") null null))`},
		{`(import (xml @native:xml/v1) (list (xml/document? (xml/document (xml/element (xml/name "" "root") (list) (list)))) (xml/element? (xml/element (xml/name "" "root") (list) (list))) (xml/name? (xml/name "urn:example" "item")) (xml/attribute? (xml/attribute (xml/name "" "id") "1")) (xml/text? (xml/text "text"))))`, `(#t #t #t #t #t)`},
		{`(import (xml @native:xml/v1) (list (xml/document? '(xml/document (xml/text "not-root"))) (xml/element? '(xml/element (xml/name "" "root") () ((xml/attribute (xml/name "" "id") "1")))) (xml/name? '(xml/name "" "bad:name")) (xml/attribute? '(xml/attribute (xml/name "" "id") 1)) (xml/text? '(xml/text "text" extra))))`, `(#f #f #f #f #f)`},
		{`(import (xml @native:xml/v1) (xml/decode "<x:root xmlns:x='urn:example' id='1' x:flag='yes'>before<x:child/>after</x:root>"))`, `(xml/document (xml/element (xml/name "urn:example" "root") ((xml/attribute (xml/name "" "id") "1") (xml/attribute (xml/name "urn:example" "flag") "yes")) ((xml/text "before") (xml/element (xml/name "urn:example" "child") null null) (xml/text "after"))))`},
		{`(import (xml @native:xml/v1) (xml/decode "<root xmlns='urn:example'><child/></root>"))`, `(xml/document (xml/element (xml/name "urn:example" "root") null ((xml/element (xml/name "urn:example" "child") null null))))`},
		{`(import (xml @native:xml/v1) (xml/decode "<?xml version='1.0'?><!-- ignored --><root>a<![CDATA[b]]>c<!-- ignored --></root>"))`, `(xml/document (xml/element (xml/name "" "root") null ((xml/text "abc"))))`},
		{`(import (xml @native:xml/v1) (xml/encode (xml/document (xml/element (xml/name "" "root") (list (xml/attribute (xml/name "" "id") "1")) (list (xml/text "a & b"))))))`, `"<root id=\"1\">a &amp; b</root>"`},
		{`(import (xml @native:xml/v1) (= (xml/decode "<x:root xmlns:x='urn:example' x:id='1'><x:child/>text</x:root>") (xml/decode (xml/encode (xml/decode "<x:root xmlns:x='urn:example' x:id='1'><x:child/>text</x:root>")))))`, `#t`},
		{`(import (xml @native:xml/v1) (let ((element (xml/element (xml/name "" "item") (list (xml/attribute (xml/name "" "id") "first") (xml/attribute (xml/name "" "other") "value")) (list (xml/text "body"))))) (list (xml/document/root (xml/document element)) (xml/element/name element) (xml/element/attributes element) (xml/element/children element) (xml/attribute/name (xml/attribute (xml/name "" "id") "first")) (xml/attribute/value (xml/attribute (xml/name "" "id") "first")) (xml/text/value (xml/text "body")) (xml/element/attributes/get element (xml/name "" "id")) (xml/element/attributes/get element (xml/name "" "missing")))))`, `((xml/element (xml/name "" "item") ((xml/attribute (xml/name "" "id") "first") (xml/attribute (xml/name "" "other") "value")) ((xml/text "body"))) (xml/name "" "item") ((xml/attribute (xml/name "" "id") "first") (xml/attribute (xml/name "" "other") "value")) ((xml/text "body")) (xml/name "" "id") "first" "body" "first" null)`},
		{`(import (xml @native:xml/v1) (taint/secret? (xml/decode (taint/secret/mark "<root id='secret'>secret</root>"))))`, `#t`},
		{`(import (xml @native:xml/v1) (taint/secret? (xml/element/name (xml/document/root (xml/decode (taint/secret/mark "<root/>"))))))`, `#t`},
		{`(import (xml @native:xml/v1) (list (error? (error/catch (xml/document (xml/text "not-root")))) (error? (error/catch (xml/element (xml/name "" "root") (list (xml/text "not-attribute")) (list)))) (error? (error/catch (xml/element (xml/name "" "root") (list) (list (xml/attribute (xml/name "" "id") "not-child"))))) (error? (error/catch (xml/name "" "bad:name"))) (error? (error/catch (xml/encode (xml/element (xml/name "" "root") (list) (list)))))))`, `(#t #t #t #t #t)`},
		{`(import (xml @native:xml/v1) (list (error? (error/catch (xml/decode "<root>"))) (error? (error/catch (xml/decode "<root/><?instruction value?>"))) (error? (error/catch (xml/decode "<!DOCTYPE root><root/>"))) (error? (error/catch (xml/decode "<root/><other/>"))) (error? (error/catch (xml/decode "<root>&unknown;</root>")))))`, `(#t #t #t #t #t)`},
	} {
		err, result := Run(test.source)
		if err != nil {
			t.Fatalf("Run(%s): %v", test.source, err)
		}
		if got := result.String(); got != test.want {
			t.Fatalf("Run(%s) = %s, want %s", test.source, got, test.want)
		}
	}
}

func TestNativeXMLTaintPropagation(t *testing.T) {
	secret := withTaint(stringValue("<root id='secret'>secret</root>"), TaintSecret)
	err, document := nativeXMLDecode(nil, []Expr{secret})
	if err != nil || TaintOf(document) != TaintSecret {
		t.Fatalf("nativeXMLDecode() = %v, %v; want secret document", err, document)
	}
	err, root := nativeXMLDocumentRoot(nil, []Expr{document})
	if err != nil || TaintOf(root) != TaintSecret {
		t.Fatalf("nativeXMLDocumentRoot() = %v, %v; want secret root", err, root)
	}
	err, name := nativeXMLElementName(nil, []Expr{root})
	if err != nil || TaintOf(name) != TaintSecret {
		t.Fatalf("nativeXMLElementName() = %v, %v; want secret name", err, name)
	}
	err, value := nativeXMLElementAttributesGet(nil, []Expr{root, list([]Expr{symbol("xml/name"), stringValue(""), stringValue("id")})})
	if err != nil || TaintOf(value) != TaintSecret {
		t.Fatalf("nativeXMLElementAttributesGet() = %v, %v; want secret value", err, value)
	}
	err, encoded := nativeXMLEncode(nil, []Expr{document})
	if err != nil || TaintOf(encoded) != TaintSecret {
		t.Fatalf("nativeXMLEncode() = %v, %v; want secret XML", err, encoded)
	}
}

func TestNativeXMLDecodeLimits(t *testing.T) {
	depth := strings.Repeat("<node>", nativeXMLMaximumDepth+1) + strings.Repeat("</node>", nativeXMLMaximumDepth+1)
	attributes := "<node" + strings.Repeat(" a='value'", nativeXMLMaximumAttributes+1) + "/>"
	nodes := "<root>" + strings.Repeat("<node/>", nativeXMLMaximumNodes) + "</root>"
	text := "<node>" + strings.Repeat("a", nativeXMLMaximumTextBytes+1) + "</node>"
	input := strings.Repeat(" ", nativeXMLMaximumInputBytes+1)
	for _, text := range []string{depth, attributes, nodes, text, input} {
		err, result := nativeXMLDecode(nil, []Expr{stringValue(text)})
		if err == nil || result != nil {
			t.Fatalf("nativeXMLDecode accepted input exceeding a parser limit")
		}
	}
}

func TestNativeXMLDecodeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err, _ := nativeXMLDecode(newEvaluator(ctx), []Expr{stringValue("<root/>")})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("nativeXMLDecode() = %v, want interrupted cancellation", err)
	}
}
