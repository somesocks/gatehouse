let Grammar = ../dhall-codegen/grammar.dhall

let Document = Grammar.Document

let s = Grammar.Schema

let text = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{=}

let integer = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{=}

let empty =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{=}

let failure =
      s.record.from
        s.record.props::{
        , required = toMap { code = text }
        , optional = toMap { message = text }
        }
        s.record.meta::{=}

let WorkspaceRef =
      s.record.from
        s.record.props::{ required = toMap { id = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{ name = Some "TranscriptWorkspaceRef" }

let SessionRef =
      s.record.from
        s.record.props::{
        , required = toMap { id = text, workspace = s.reference.from s.reference.props::{ to = "TranscriptWorkspaceRef" } s.reference.meta::{=} }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "TranscriptSessionRef" }

let EventRef =
      s.record.from
        s.record.props::{
        , required = toMap { id = text, session = s.reference.from s.reference.props::{ to = "TranscriptSessionRef" } s.reference.meta::{=} }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "TranscriptEventRef" }

let AgentRef =
      s.record.from
        s.record.props::{
        , required = toMap { id = text, workspace = s.reference.from s.reference.props::{ to = "TranscriptWorkspaceRef" } s.reference.meta::{=} }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "TranscriptAgentRef" }

let PrincipalRef =
      s.record.from
        s.record.props::{ required = toMap { id = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{ name = Some "TranscriptPrincipalRef" }

let Principal =
      s.record.from
        s.record.props::{
        , required = toMap { enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{=}, ref = s.reference.from s.reference.props::{ to = "TranscriptPrincipalRef" } s.reference.meta::{=} }
        , optional = toMap { alias = text, name = text }
        }
        s.record.meta::{ name = Some "TranscriptPrincipal" }

let GatewayRef =
      s.record.from
        s.record.props::{ required = toMap { id = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{ name = Some "TranscriptGatewayRef" }

let Metrics =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional = toMap { request_ms = integer, input_tokens = integer, cached_input_tokens = integer, output_tokens = integer, reasoning_tokens = integer, total_tokens = integer }
        }
        s.record.meta::{ name = Some "TranscriptEventMetrics" }

let File =
      s.record.from
        s.record.props::{
        , required = toMap { id = text, name = text, size = integer, fingerprint = text }
        , optional = toMap { media_type = text }
        }
        s.record.meta::{ name = Some "TranscriptFile" }

let attachments =
      s.list.from
        s.list.props::{ values = s.reference.from s.reference.props::{ to = "TranscriptFile" } s.reference.meta::{=} }
        s.list.meta::{=}

let message =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional = toMap { text = text, agents = s.list.from s.list.props::{ values = text } s.list.meta::{=}, attachments = attachments }
        }
        s.record.meta::{=}

let agentSuccess =
      s.record.from
        s.record.props::{ required = toMap { text = text }, optional = toMap { attachments = attachments } }
        s.record.meta::{=}

let toolRequest =
      s.record.from
        s.record.props::{ required = toMap { name = text, reason = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{=}

let toolFailure =
      s.record.from
        s.record.props::{ required = toMap { name = text, call_id = text, code = text, output = text }, optional = toMap { message = text } }
        s.record.meta::{=}

let description =
      s.record.from
        s.record.props::{ required = toMap { description = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{=}

let thinkingUpdate =
      s.record.from
        s.record.props::{ required = toMap { reason = text }, optional = toMap { until = text } }
        s.record.meta::{=}

let other =
      s.record.from
        s.record.props::{ required = toMap { original_kind = text }, optional = [] : List { mapKey : Text, mapValue : s.type } }
        s.record.meta::{=}

let event =
      \(kind : Text) ->
      \(payload : s.type) ->
        s.record.from
          s.record.props::{
          , required =
              toMap
                { created_at = text
                , kind = s.text.from s.text.props::{ variant = s.text.variants.literal kind } s.text.meta::{=}
                , payload = payload
                , ref = s.reference.from s.reference.props::{ to = "TranscriptEventRef" } s.reference.meta::{=}
                }
          , optional =
              toMap
                { parent = s.reference.from s.reference.props::{ to = "TranscriptEventRef" } s.reference.meta::{=}
                , author_principal = s.reference.from s.reference.props::{ to = "TranscriptPrincipal" } s.reference.meta::{=}
                , author_agent = s.reference.from s.reference.props::{ to = "TranscriptAgentRef" } s.reference.meta::{=}
                , author_gateway = s.reference.from s.reference.props::{ to = "TranscriptGatewayRef" } s.reference.meta::{=}
                , metrics = s.reference.from s.reference.props::{ to = "TranscriptEventMetrics" } s.reference.meta::{=}
                }
          }
          s.record.meta::{=}

let TranscriptEvent =
      s.oneOf.from
        { options =
            [ event "message.text" message
            , event "agent.request" (s.record.from s.record.props::{ required = toMap { agent = text }, optional = [] : List { mapKey : Text, mapValue : s.type } } s.record.meta::{=})
            , event "agent.success" agentSuccess
            , event "agent.failure" failure
            , event "thinking.request" empty
            , event "thinking.update" thinkingUpdate
            , event "thinking.success" empty
            , event "thinking.failure" failure
            , event "tool.request" toolRequest
            , event "tool.success" empty
            , event "tool.failure" toolFailure
            , event "approval.request" description
            , event "approval.success" empty
            , event "approval.failure" failure
            , event "input.request" description
            , event "input.success" empty
            , event "input.failure" failure
            , event "cancel.request" empty
            , event "cancel.success" empty
            , event "cancel.failure" failure
            , event "other" other
            ]
        }
        s.oneOf.meta::{ name = Some "TranscriptEvent" }

in  Document::{
    , headers = [] : List Text
    , schemas =
        [ s.root.from WorkspaceRef s.root.meta::{ name = "TranscriptWorkspaceRef" }
        , s.root.from SessionRef s.root.meta::{ name = "TranscriptSessionRef" }
        , s.root.from EventRef s.root.meta::{ name = "TranscriptEventRef" }
        , s.root.from AgentRef s.root.meta::{ name = "TranscriptAgentRef" }
        , s.root.from PrincipalRef s.root.meta::{ name = "TranscriptPrincipalRef" }
        , s.root.from Principal s.root.meta::{ name = "TranscriptPrincipal" }
        , s.root.from GatewayRef s.root.meta::{ name = "TranscriptGatewayRef" }
        , s.root.from Metrics s.root.meta::{ name = "TranscriptEventMetrics" }
        , s.root.from File s.root.meta::{ name = "TranscriptFile" }
        , s.root.from TranscriptEvent s.root.meta::{ name = "TranscriptEvent" }
        ]
    }
