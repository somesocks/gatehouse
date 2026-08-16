let Grammar = ./dhall-codegen/grammar.dhall

let Document = Grammar.Document

let s = Grammar.Schema

let WorkspaceRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable workspace identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "WorkspaceRef"
        , description = Some "The stable identity of a workspace."
        }

let Workspace =
      s.record.from
        s.record.props::{
        , required =
            toMap
               { ref =
                   s.reference.from
                     s.reference.props::{ to = "WorkspaceRef" }
                     s.reference.meta::{ description = Some "workspace identity" }
               , enabled =
                   s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the workspace is enabled" }
              }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace reconciliation alias" }
              , name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace display name" }
              }
        }
        s.record.meta::{
        , name = Some "Workspace"
        , description = Some "A Gatehouse tenant and hard security boundary."
        }

let PrincipalRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                      s.text.meta::{ description = Some "durable typed principal identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "PrincipalRef"
        , description = Some "The durable identity of a principal."
        }

let Principal =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "principal identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the principal is enabled" }
              }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "principal reconciliation alias" }
              , name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "principal display name" }
              }
        }
        s.record.meta::{
        , name = Some "Principal"
        , description = Some "A deployment-level human or service actor."
        }

let Verifiers =
      s.list.from
        s.list.props::{
        , values =
            s.any.from
              s.any.props::{ variant = s.any.variants.permissive }
              s.any.meta::{ description = Some "provider-defined verifier object" }
        }
        s.list.meta::{ description = Some "verification methods that can prove control of the identity" }

let Identity =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "durable typed identity" }
              , key =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "globally namespaced login identity" }
              , principal =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "owning principal identity" }
              , verifiers = Verifiers
               , enabled =
                   s.boolean.from
                     s.boolean.props::{=}
                     s.boolean.meta::{ description = Some "whether the identity is enabled" }
               , revision =
                   s.number.from
                     s.number.props::{ variant = s.number.variants.integer }
                     s.number.meta::{ description = Some "monotonic identity configuration revision" }
               }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "identity reconciliation alias" }
              }
        }
        s.record.meta::{
        , name = Some "Identity"
        , description = Some "A verified authentication binding for a principal."
        }

let KeychainRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable keychain identity" }
              , version =
                   s.number.from
                     s.number.props::{ variant = s.number.variants.integer }
                     s.number.meta::{ description = Some "keychain encryption key version" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "KeychainRef"
        , description = Some "The stable identity of a keychain encryption key version."
        }

let Keychain =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "KeychainRef" }
                    s.reference.meta::{ description = Some "keychain key identity" }
               , kek_kdf =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "key-encryption-key derivation descriptor" }
              , key =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "encrypted keychain encryption key" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the key version is selected for new encryption" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "Keychain"
        , description = Some "A versioned keychain encryption key."
        }

let AgentProviderRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed agent provider identity" } }
		, optional = toMap
			{ alias = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent provider reconciliation alias" } }
        }
        s.record.meta::{ name = Some "AgentProviderRef", description = Some "The stable identity of an agent provider." }

let AgentProvider =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "AgentProviderRef" } s.reference.meta::{ description = Some "provider identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "provider configuration revision" }
            , protocol = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider protocol" }
            , base_url = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider base URL" }
            , keychain = s.reference.from s.reference.props::{ to = "KeychainRef" } s.reference.meta::{ description = Some "API key encryption key" }
            , api_key = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "encrypted API key" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the provider is enabled" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "AgentProvider", description = Some "A configured model-provider endpoint." }

let AgentModelRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed agent model identity" } }
		, optional = toMap
			{ alias = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent model reconciliation alias" } }
        }
        s.record.meta::{ name = Some "AgentModelRef", description = Some "The stable identity of an agent model." }

let AgentModel =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "AgentModelRef" } s.reference.meta::{ description = Some "model identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "model configuration revision" }
            , provider = s.reference.from s.reference.props::{ to = "AgentProviderRef" } s.reference.meta::{ description = Some "provider identity" }
            , model = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider model identifier" }
            , parameters = s.any.from s.any.props::{ variant = s.any.variants.permissive } s.any.meta::{ description = Some "provider model parameters" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the model is enabled" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "AgentModel", description = Some "A configured provider model." }

let WorkspaceAgentRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { workspace = s.reference.from s.reference.props::{ to = "WorkspaceRef" } s.reference.meta::{ description = Some "owning workspace identity" }
            , model = s.reference.from s.reference.props::{ to = "AgentModelRef" } s.reference.meta::{ description = Some "agent model identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "WorkspaceAgentRef", description = Some "The stable identity of a workspace agent binding." }

let WorkspaceAgent =
      s.record.from
        s.record.props::{
        , required = toMap
              { ref = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent identity" }
              , priority = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "agent selection priority" }
              , max_turns = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "maximum tool-using model turns per reply" }
              , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the workspace agent is enabled" }
              }
        , optional = toMap
              { label = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent display label" }
              }
        }
        s.record.meta::{ name = Some "WorkspaceAgent", description = Some "A model assigned to a workspace." }

let GroupRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceRef" }
                    s.reference.meta::{ description = Some "owning workspace identity" }
              , id =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                      s.text.meta::{ description = Some "durable typed group identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GroupRef"
        , description = Some "The stable identity of a workspace group."
        }

let Group =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "GroupRef" }
                    s.reference.meta::{ description = Some "group identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the group is enabled" }
              }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "group reconciliation alias" }
              , name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "group display name" }
              }
        }
        s.record.meta::{
        , name = Some "Group"
        , description = Some "A workspace-local authorization group."
        }

let GroupMember =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { group =
                  s.reference.from
                    s.reference.props::{ to = "GroupRef" }
                    s.reference.meta::{ description = Some "assigned group identity" }
              , principal =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "assigned principal identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the group assignment is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GroupMember"
        , description = Some "A principal assigned to a workspace-local authorization group."
        }

let SessionRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceRef" }
                    s.reference.meta::{ description = Some "owning workspace identity" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "durable typed session identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "SessionRef"
        , description = Some "The stable identity of a workspace session."
        }

let Session =
      s.record.from
        s.record.props::{
        , required =
            toMap
               { ref =
                   s.reference.from
                     s.reference.props::{ to = "SessionRef" }
                     s.reference.meta::{ description = Some "session identity" }
               , created_at =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "session creation timestamp" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the session is enabled" }
              }
        , optional =
            toMap
              { name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "AI-generated session title" }
              , author_principal =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "principal session author" }
              , author_agent =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceAgentRef" }
                    s.reference.meta::{ description = Some "workspace agent session author" }
              , author_gateway =
                  s.reference.from
                    s.reference.props::{ to = "GatewayRef" }
                    s.reference.meta::{ description = Some "gateway session author" }
              }
        }
        s.record.meta::{
        , name = Some "Session"
        , description = Some "A durable workspace session."
        }

let StorageObjectRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed storage object identity" } }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "StorageObjectRef", description = Some "The stable identity of a stored object." }

let StorageObject =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "StorageObjectRef" } s.reference.meta::{ description = Some "storage object identity" }
            , state = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "storage object lifecycle state" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "storage object creation timestamp" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "StorageObject", description = Some "An immutable object stored by a configured provider." }

let SessionFileRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { session = s.reference.from s.reference.props::{ to = "SessionRef" } s.reference.meta::{ description = Some "owning session identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed session file identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionFileRef", description = Some "The stable identity of a file attached to a session." }

let SessionFile =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SessionFileRef" } s.reference.meta::{ description = Some "session file identity" }
            , storage_object = s.reference.from s.reference.props::{ to = "StorageObjectRef" } s.reference.meta::{ description = Some "backing immutable storage object" }
            , name = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "client file name" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session file creation timestamp" }
            }
        , optional = toMap
            { media_type = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "declared media type" } }
        }
        s.record.meta::{ name = Some "SessionFile", description = Some "A private file attached to a session." }

let GatewayRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed gateway process identity" } }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "GatewayRef", description = Some "The runtime identity of a gateway process." }

let SessionEventRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { session =
                  s.reference.from
                    s.reference.props::{ to = "SessionRef" }
                    s.reference.meta::{ description = Some "owning session identity" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "durable typed session event identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionEventRef", description = Some "The stable identity of a session event." }

let SessionEvent =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "SessionEventRef" }
                    s.reference.meta::{ description = Some "event identity" }
              , kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "event kind" }
              , payload =
                  s.map.from
                    s.map.props::{
                    , keys = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "payload property name" }
                    , values = s.any.from s.any.props::{ variant = s.any.variants.permissive } s.any.meta::{ description = Some "payload property value" }
                    , variant = s.map.variants.none
                    }
                    s.map.meta::{ description = Some "event payload" }
              , created_at =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "event creation timestamp" }
              }
        , optional =
            toMap
              { parent =
                  s.reference.from
                    s.reference.props::{ to = "SessionEventRef" }
                    s.reference.meta::{ description = Some "parent session event" }
              , author_principal =
                  s.reference.from
                    s.reference.props::{ to = "Principal" }
                    s.reference.meta::{ description = Some "principal event author" }
              , author_agent =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceAgentRef" }
                    s.reference.meta::{ description = Some "workspace agent event author" }
              , author_gateway =
                  s.reference.from
                    s.reference.props::{ to = "GatewayRef" }
                    s.reference.meta::{ description = Some "gateway event author" }
              }
        }
        s.record.meta::{ name = Some "SessionEvent", description = Some "An immutable event in a durable workspace session." }

let ActivityEventRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceRef" }
                    s.reference.meta::{ description = Some "owning workspace identity" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "durable typed activity identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ActivityEventRef", description = Some "The stable identity of an activity event." }

let ActivityEvent =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "ActivityEventRef" }
                    s.reference.meta::{ description = Some "activity event identity" }
              , event =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "activity event type" }
              , resource_kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "subject resource kind" }
              , created_at =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "activity creation timestamp" }
              }
        , optional =
            toMap
              { session =
                  s.reference.from
                    s.reference.props::{ to = "SessionRef" }
                    s.reference.meta::{ description = Some "session activity subject" }
              , session_event =
                  s.reference.from
                    s.reference.props::{ to = "SessionEventRef" }
                    s.reference.meta::{ description = Some "session event activity subject" }
              }
        }
        s.record.meta::{ name = Some "ActivityEvent", description = Some "A durable workspace activity event with a typed resource subject." }

let ActivityCursor =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { created_at =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "activity checkpoint timestamp" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "activity checkpoint identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ActivityCursor", description = Some "An activity topic checkpoint." }

let ActivityTopicCheckpoint =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { topic =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "opaque activity topic" }
              }
        , optional =
            toMap
              { cursor =
                  s.reference.from
                    s.reference.props::{ to = "ActivityCursor" }
                    s.reference.meta::{ description = Some "latest observed activity checkpoint" }
              }
        }
        s.record.meta::{ name = Some "ActivityTopicCheckpoint", description = Some "A topic and its latest observed activity checkpoint." }

let ActivityTopicCheckpoints =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { topics =
                  s.list.from
                    s.list.props::{
                    , values =
                        s.reference.from
                          s.reference.props::{ to = "ActivityTopicCheckpoint" }
                          s.reference.meta::{ description = Some "requested or observed topic checkpoints" }
                    }
                    s.list.meta::{ description = Some "activity topic checkpoints" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ActivityTopicCheckpoints", description = Some "A collection of activity topic checkpoints." }

let SessionPrincipalGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { session =
                  s.reference.from
                    s.reference.props::{ to = "SessionRef" }
                    s.reference.meta::{ description = Some "granted session identity" }
              , principal =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "granted principal identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the principal grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "SessionPrincipalGrant"
        , description = Some "A principal granted access to a session."
        }

let SessionGroupGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { session =
                  s.reference.from
                    s.reference.props::{ to = "SessionRef" }
                    s.reference.meta::{ description = Some "granted session identity" }
              , group =
                  s.reference.from
                    s.reference.props::{ to = "GroupRef" }
                    s.reference.meta::{ description = Some "granted group identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the group grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "SessionGroupGrant"
        , description = Some "A workspace group granted access to a session."
        }

let ToolRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceRef" }
                    s.reference.meta::{ description = Some "owning workspace identity" }
              , id =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                      s.text.meta::{ description = Some "durable typed tool identity" }
              }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local tool reconciliation alias" }
              }
        }
        s.record.meta::{
        , name = Some "ToolRef"
        , description = Some "The stable identity of a workspace tool."
        }

let Tool =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "ToolRef" }
                    s.reference.meta::{ description = Some "tool identity" }
              , source =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "tool module source" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the tool is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "Tool"
        , description = Some "A workspace-bound Lisp tool module."
        }

let ResourceRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.reference.from
                    s.reference.props::{ to = "WorkspaceRef" }
                    s.reference.meta::{ description = Some "owning workspace identity" }
              , id =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                      s.text.meta::{ description = Some "durable typed resource identity" }
              }
        , optional =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local resource reconciliation alias" }
              }
        }
        s.record.meta::{
        , name = Some "ResourceRef"
        , description = Some "The stable identity of a workspace resource."
        }

let Resource =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref =
                  s.reference.from
                    s.reference.props::{ to = "ResourceRef" }
                    s.reference.meta::{ description = Some "resource identity" }
              , source =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "resource value source" }
              , secret =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource value is secret" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "Resource"
        , description = Some "A workspace-bound Lisp resource value."
        }

let GroupToolGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { group =
                  s.reference.from
                    s.reference.props::{ to = "GroupRef" }
                    s.reference.meta::{ description = Some "granted group identity" }
              , tool =
                  s.reference.from
                    s.reference.props::{ to = "ToolRef" }
                    s.reference.meta::{ description = Some "granted tool identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the tool grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GroupToolGrant"
        , description = Some "A workspace group granted a tool."
        }

let GroupResourceGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { group =
                  s.reference.from
                    s.reference.props::{ to = "GroupRef" }
                    s.reference.meta::{ description = Some "granted group identity" }
              , resource =
                  s.reference.from
                    s.reference.props::{ to = "ResourceRef" }
                    s.reference.meta::{ description = Some "granted resource identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GroupResourceGrant"
        , description = Some "A workspace group granted a resource."
        }

in  Document::{
    , headers = [] : List Text
    , schemas =
          [ s.root.from WorkspaceRef s.root.meta::{ name = "WorkspaceRef" }
          , s.root.from Workspace s.root.meta::{ name = "Workspace" }
          , s.root.from PrincipalRef s.root.meta::{ name = "PrincipalRef" }
          , s.root.from Principal s.root.meta::{ name = "Principal" }
          , s.root.from Identity s.root.meta::{ name = "Identity" }
          , s.root.from KeychainRef s.root.meta::{ name = "KeychainRef" }
           , s.root.from Keychain s.root.meta::{ name = "Keychain" }
           , s.root.from AgentProviderRef s.root.meta::{ name = "AgentProviderRef" }
           , s.root.from AgentProvider s.root.meta::{ name = "AgentProvider" }
            , s.root.from AgentModelRef s.root.meta::{ name = "AgentModelRef" }
           , s.root.from AgentModel s.root.meta::{ name = "AgentModel" }
           , s.root.from WorkspaceAgentRef s.root.meta::{ name = "WorkspaceAgentRef" }
           , s.root.from WorkspaceAgent s.root.meta::{ name = "WorkspaceAgent" }
          , s.root.from ResourceRef s.root.meta::{ name = "ResourceRef" }
          , s.root.from ToolRef s.root.meta::{ name = "ToolRef" }
           , s.root.from GroupRef s.root.meta::{ name = "GroupRef" }
           , s.root.from Group s.root.meta::{ name = "Group" }
         , s.root.from GroupMember s.root.meta::{ name = "GroupMember" }
           , s.root.from SessionRef s.root.meta::{ name = "SessionRef" }
           , s.root.from Session s.root.meta::{ name = "Session" }
           , s.root.from StorageObjectRef s.root.meta::{ name = "StorageObjectRef" }
           , s.root.from StorageObject s.root.meta::{ name = "StorageObject" }
           , s.root.from SessionFileRef s.root.meta::{ name = "SessionFileRef" }
           , s.root.from SessionFile s.root.meta::{ name = "SessionFile" }
            , s.root.from GatewayRef s.root.meta::{ name = "GatewayRef" }
            , s.root.from SessionEventRef s.root.meta::{ name = "SessionEventRef" }
            , s.root.from SessionEvent s.root.meta::{ name = "SessionEvent" }
          , s.root.from ActivityEventRef s.root.meta::{ name = "ActivityEventRef" }
          , s.root.from ActivityEvent s.root.meta::{ name = "ActivityEvent" }
          , s.root.from ActivityCursor s.root.meta::{ name = "ActivityCursor" }
          , s.root.from ActivityTopicCheckpoint s.root.meta::{ name = "ActivityTopicCheckpoint" }
          , s.root.from ActivityTopicCheckpoints s.root.meta::{ name = "ActivityTopicCheckpoints" }
          , s.root.from SessionPrincipalGrant s.root.meta::{ name = "SessionPrincipalGrant" }
         , s.root.from SessionGroupGrant s.root.meta::{ name = "SessionGroupGrant" }
         , s.root.from Tool s.root.meta::{ name = "Tool" }
        , s.root.from Resource s.root.meta::{ name = "Resource" }
        , s.root.from GroupToolGrant s.root.meta::{ name = "GroupToolGrant" }
        , s.root.from GroupResourceGrant s.root.meta::{ name = "GroupResourceGrant" }
        ]
    }
