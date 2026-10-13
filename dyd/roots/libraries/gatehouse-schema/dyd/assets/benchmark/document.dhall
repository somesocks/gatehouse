let Grammar = ../dhall-codegen/grammar.dhall

let s = Grammar.Schema

let Definitions = ./definitions.dhall

let Config = s.root.from
      (s.record.from
        s.record.props::{ required = toMap Definitions.fields }
        s.record.meta::{ description = Some "Gatehouse benchmark configuration" })
      s.root.meta::{ name = "GatehouseBenchmarkConfig" }

in Grammar.Document::{ schemas = Definitions.schemas # [ Config ] }
