let Grammar = ../dhall-codegen/grammar.dhall

let s = Grammar.Schema

let text =
      \(description : Text) ->
        s.text.from s.text.props::{ variant = s.text.variants.none }
          s.text.meta::{ description = Some description }

let integer =
      \(description : Text) ->
        s.number.from s.number.props::{ variant = s.number.variants.integer }
          s.number.meta::{ description = Some description }

let boolean = s.boolean.from s.boolean.props::{=} s.boolean.meta::{=}

let ref =
      \(definition : s.root.type) ->
        s.reference.from { to = definition.meta.name } s.reference.meta::{=}

let list = \(value : s.type) -> s.list.from { values = value } s.list.meta::{=}

let APIKey = s.record.from
      s.record.props::{
      , required = toMap { sources = list (text "deferred credential source") }
      , optional = toMap { keychain = text "keychain identifier" }
      }
      s.record.meta::{=}

let Provider = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap { alias = text "provider alias", protocol = text "provider protocol" }
        , optional = toMap
            { base_url = text "provider base URL"
            , api_key = APIKey
            , enabled = boolean
            }
          }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkProvider" }

let Compaction = s.record.from
      s.record.props::{
      , required = toMap
          { algorithm = s.text.from
              s.text.props::{ variant = s.text.variants.literal "mcmtr" }
              s.text.meta::{=}
          }
      , optional = toMap
          { history_bytes = integer "transcript history capacity in bytes"
          , buffer_bytes = integer "transcript buffer capacity in bytes"
          }
      }
      s.record.meta::{=}

let Model = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap
            { alias = text "model alias"
            , provider = text "provider alias"
            , model = text "provider model identifier"
            , parameters = s.any.from s.any.props::{ variant = s.any.variants.permissive }
                s.any.meta::{ description = Some "provider model parameters" }
            }
        , optional = toMap
            { compaction = Compaction
            , enabled = boolean
            , max_output_tokens = integer "maximum output tokens per request"
            , max_turns = integer "maximum tool-using turns"
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkModel" }

let Prompt = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap { alias = text "prompt alias", mode = text "builtin, append, or replace" }
        , optional = toMap { text = text "prompt text or file: reference" }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkPrompt" }

let Question = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap { alias = text "question alias", text = text "question text or file: reference" }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkQuestion" }

let Environment = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap { alias = text "Lisp environment alias" }
        , optional = toMap
            { project_linked = boolean
            , prelude = text "Lisp prelude or file: reference; omitted uses the built-in prelude"
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkEnvironment" }

let Matrix = s.record.from
      s.record.props::{
      , required = toMap
          { models = list (text "model alias")
          , system_prompts = list (text "prompt alias")
          , questions = list (text "question alias")
          , lisp_environments = list (text "environment alias")
          }
      }
      s.record.meta::{=}

let Test = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap { alias = text "test alias", matrix = Matrix }
        , optional = toMap { repetitions = integer "positive repetition count; defaults to one" }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "BenchmarkTest" }

let fields =
      { api_version = s.text.from
          s.text.props::{ variant = s.text.variants.literal "v1" }
          s.text.meta::{=}
      , agent_providers = list (ref Provider)
      , agent_models = list (ref Model)
      , system_prompts = list (ref Prompt)
      , questions = list (ref Question)
      , lisp_environments = list (ref Environment)
      , tests = list (ref Test)
      }

in { fields
   , Provider
   , Model
   , Prompt
   , Question
   , Environment
   , ref
   , list
   , fileReference = text "file: reference to one YAML definition"
   , schemas = [ Provider, Model, Prompt, Question, Environment, Test ]
   }
