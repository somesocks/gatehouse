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

let object = s.map.from
      s.map.props::{
      , keys = text "property name"
      , values = s.any.from s.any.props::{ variant = s.any.variants.permissive }
          s.any.meta::{=}
      }
      s.map.meta::{=}

let snapshotFields =
      { id = text "snapshot identity generated before persistence"
      , execution_id = text "owning execution identity"
      , alias = text "input alias within this execution"
      }

let Execution = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap
            { id = text "execution identity generated before persistence"
            , config_path = text "benchmark configuration source location"
            , config_api_version = text "benchmark configuration API version"
            , gatehouse_version = text "Gatehouse version used for this execution"
            , gatehouse_fingerprint = text "Gatehouse build fingerprint"
            , started_at = text "execution start timestamp in UTC RFC 3339"
            , status = text "execution state: running, completed, failed, or cancelled"
            }
        , optional = toMap
            { finished_at = text "execution finish timestamp in UTC RFC 3339"
            , error = text "execution infrastructure error, when present"
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "Execution" }

let Provider = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap (snapshotFields // { protocol = text "declared provider protocol" })
        , optional = toMap
            { endpoint = text "resolved complete provider request URL"
            , api_key_sources = s.list.from
                { values = text "deferred credential source descriptor" }
                s.list.meta::{=}
            , keychain = text "declared credential keychain identifier"
            , enabled = boolean
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "Provider" }

let Model = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap (snapshotFields //
            { provider_id = text "provider snapshot identity in the same execution"
            , model = text "declared provider model identifier"
            , parameters = object
            })
        , optional = toMap
            { max_turns = integer "declared tool-using turn limit"
            , max_output_tokens = integer "declared output token limit"
            , compaction_algorithm = text "declared compaction algorithm; omitted means no compaction record supplied"
            , history_bytes = integer "declared transcript history capacity"
            , buffer_bytes = integer "declared transcript buffer capacity"
            , enabled = boolean
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "Model" }

let SystemPrompt = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap (snapshotFields // { mode = text "declared prompt mode: builtin, append, or replace" })
        , optional = toMap { text = text "loaded prompt text; absent for builtin mode" }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "SystemPrompt" }

let Question = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap (snapshotFields // { text = text "loaded question text" })
        }
        s.record.meta::{=})
      s.root.meta::{ name = "Question" }

let LispEnvironment = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap snapshotFields
        , optional = toMap
            { project_linked = boolean
            , prelude = text "loaded prelude; absence and explicitly empty text remain distinct"
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "LispEnvironment" }

let Run = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap
            { id = text "run identity generated before persistence"
            , execution_id = text "owning execution identity"
            , test_alias = text "test definition alias within the execution"
            , model_id = text "selected model snapshot identity"
            , system_prompt_id = text "selected prompt snapshot identity"
            , question_id = text "selected question snapshot identity"
            , lisp_environment_id = text "selected environment snapshot identity"
            , repetition = integer "one-based repetition ordinal"
            , status = text "execution state: planned, running, completed, failed, or cancelled; not a quality judgment"
            }
        , optional = toMap
            { workspace_id = text "original runtime workspace identity"
            , workspace_agent_id = text "original runtime workspace-agent identity"
            , session_id = text "original runtime session identity"
            , agent_request_id = text "original agent-request event identity within this run"
            , terminal_event_id = text "original terminal event identity within this run"
            , started_at = text "run start timestamp in UTC RFC 3339"
            , finished_at = text "run finish timestamp in UTC RFC 3339"
            , error = text "run execution error, when present"
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "Run" }

let SessionEvent = s.root.from
      (s.record.from
        s.record.props::{
        , required = toMap
            { id = text "original native session-event identity"
            , run_id = text "owning benchmark run identity"
            , workspace = text "original runtime workspace identity"
            , session = text "original runtime session identity"
            , kind = text "original native event kind"
            , payload = object
            , created_at = text "original event creation timestamp in UTC RFC 3339"
            }
        , optional = toMap
            { parent = text "original parent event identity within the same run"
            , author_principal = text "original principal author identity"
            , author_agent = text "original workspace-agent author identity"
            , author_gateway = text "original gateway author identity"
            , metrics = object
            }
        }
        s.record.meta::{=})
      s.root.meta::{ name = "SessionEvent" }

in Grammar.Document::{
     , description = Some "Captured benchmark records, independent of configuration and operational models."
     , schemas =
         [ Execution
         , Provider
         , Model
         , SystemPrompt
         , Question
         , LispEnvironment
         , Run
         , SessionEvent
         ]
     }
