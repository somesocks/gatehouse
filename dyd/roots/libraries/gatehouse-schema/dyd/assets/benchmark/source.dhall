let Grammar = ../dhall-codegen/grammar.dhall

let s = Grammar.Schema

let Definitions = ./definitions.dhall

let input =
      \(definition : s.root.type) ->
        Definitions.list
          (s.oneOf.from
            { options = [ Definitions.fileReference, Definitions.ref definition ] }
            s.oneOf.meta::{=})

let fields = Definitions.fields //
      { agent_providers = input Definitions.Provider
      , agent_models = input Definitions.Model
      , system_prompts = input Definitions.Prompt
      , questions = input Definitions.Question
      , lisp_environments = input Definitions.Environment
      }

let Config = s.root.from
      (s.record.from
        s.record.props::{ required = toMap fields }
        s.record.meta::{ description = Some "Gatehouse benchmark configuration" })
      s.root.meta::{ name = "GatehouseBenchmarkSourceConfig" }

in Grammar.Document::{ schemas = Definitions.schemas # [ Config ] }
