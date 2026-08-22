package lisp

import "hash/crc32"

const nativeCryptoDigestCRC32ID = "native:crypto/digest/crc32/v1"

var nativeCryptoDigestCRC32Table = crc32.MakeTable(crc32.IEEE)

var nativeCryptoDigestCRC32DigestDocumentation = doc(
	"(crc32/digest bytes) -> Integer",
	"Returns the IEEE CRC-32 digest of Bytes.",
	"(import (crc32 @native:crypto/digest/crc32/v1) (crc32/digest (bytes/utf8/encode \"123456789\")))",
	"3421780262",
)

var nativeCryptoDigestCRC32UpdateDocumentation = doc(
	"(crc32/update crc bytes) -> Integer",
	"Returns the IEEE CRC-32 after appending Bytes to the prior digest, starting from zero.",
	"(import (crc32 @native:crypto/digest/crc32/v1) (crc32/update (crc32/update 0 (bytes/utf8/encode \"1234\")) (bytes/utf8/encode \"56789\")))",
	"3421780262",
)

func nativeCryptoDigestCRC32Module() Expr {
	digest := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoDigestCRC32Digest),
	}, nativeCryptoDigestCRC32DigestDocumentation.text())
	update := withHelp(&builtin{
		leaky: true,
		call:  pure(nativeCryptoDigestCRC32Update),
	}, nativeCryptoDigestCRC32UpdateDocumentation.text())
	return list([]Expr{
		symbol("quote"),
		list([]Expr{
			pairValue(symbol("digest"), digest),
			pairValue(symbol("update"), update),
		}),
	})
}

func nativeCryptoDigestCRC32Digest(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 1 {
		return expressionError("crc32/digest requires one Bytes value"), nil
	}
	err, value := requireBytes(arguments[0])
	if err != nil {
		return err, nil
	}
	return nil, integer(int64(nativeCryptoDigestCRC32UpdateValue(0, value)))
}

func nativeCryptoDigestCRC32Update(_ *evaluator, arguments []Expr) (error, Expr) {
	if len(arguments) != 2 {
		return expressionError("crc32/update requires an IEEE CRC-32 Integer and one Bytes value"), nil
	}
	err, value := requireInteger(arguments[0])
	if err != nil {
		return err, nil
	}
	if value < 0 || value > 0xffffffff {
		return expressionError("crc32/update requires an IEEE CRC-32 Integer from 0 through 4294967295"), nil
	}
	err, bytes := requireBytes(arguments[1])
	if err != nil {
		return err, nil
	}
	return nil, integer(int64(nativeCryptoDigestCRC32UpdateValue(uint32(value), bytes)))
}

func nativeCryptoDigestCRC32UpdateValue(value uint32, bytes string) uint32 {
	value = ^value
	for index := 0; index < len(bytes); index++ {
		value = nativeCryptoDigestCRC32Table[byte(value)^bytes[index]] ^ value>>8
	}
	return ^value
}
