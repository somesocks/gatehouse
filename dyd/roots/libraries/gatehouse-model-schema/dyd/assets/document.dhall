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
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable workspace agent binding identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "WorkspaceAgentRef", description = Some "The stable identity of a workspace agent binding." }

let WorkspaceAgent =
      s.record.from
        s.record.props::{
		, required = toMap
		       { ref = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent identity" }
		       , alias = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace-local binding alias" }
		        , model = s.reference.from s.reference.props::{ to = "AgentModelRef" } s.reference.meta::{ description = Some "current agent model identity" }
                , priority = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "agent selection priority" }
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
               { id =
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

let GroupMemberRef =
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
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GroupMemberRef"
        , description = Some "The stable identity of a principal's group membership."
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

let ProjectRef =
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
                    s.text.meta::{ description = Some "durable typed project identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectRef", description = Some "The stable identity of a workspace project." }

let Project =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { ref = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "project identity" }
              , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project is enabled" }
              , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project creation timestamp" }
              }
        , optional =
            toMap
              { name = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project display name" }
              , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project description" }
              }
        }
        s.record.meta::{ name = Some "Project", description = Some "A collaboration container within a workspace." }

let ProjectPrincipalGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "granted project identity" }
              , principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "granted principal identity" }
              , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectPrincipalGrant", description = Some "A principal granted access to a project." }

let ProjectGroupGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "granted project identity" }
              , group = s.reference.from s.reference.props::{ to = "GroupRef" } s.reference.meta::{ description = Some "granted group identity" }
              , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project grant is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectGroupGrant", description = Some "A group granted access to a project." }

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
               , project =
                   s.reference.from
                     s.reference.props::{ to = "ProjectRef" }
                     s.reference.meta::{ description = Some "optional project association" }
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
             , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the session file is available" }
             , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session file creation timestamp" }
             }
        , optional = toMap
            { media_type = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "declared media type" } }
        }
        s.record.meta::{ name = Some "SessionFile", description = Some "A private file attached to a session." }

let ProjectFileRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "owning project identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project file identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectFileRef", description = Some "The stable identity of a file attached to a project." }

let ProjectFile =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectFileRef" } s.reference.meta::{ description = Some "project file identity" }
            , storage_object = s.reference.from s.reference.props::{ to = "StorageObjectRef" } s.reference.meta::{ description = Some "backing immutable storage object" }
            , name = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "client file name" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project file is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project file creation timestamp" }
            }
        , optional = toMap
            { media_type = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "declared media type" } }
        }
        s.record.meta::{ name = Some "ProjectFile", description = Some "A shared file attached to a project." }

let ProjectNoteRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "owning project identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project note identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectNoteRef", description = Some "The stable identity of a note attached to a project." }

let ProjectNote =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectNoteRef" } s.reference.meta::{ description = Some "project note identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "note title" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "note description" }
            , body = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "Markdown note body" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of the current revision body are marked sensitive" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project note is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project note creation timestamp" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "current immutable note revision" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal note author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent note author" }
            , author_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway note author" }
            }
        }
        s.record.meta::{ name = Some "ProjectNote", description = Some "A shared Markdown note attached to a project." }

let ProjectNoteRevisionRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { note = s.reference.from s.reference.props::{ to = "ProjectNoteRef" } s.reference.meta::{ description = Some "revised project note identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "immutable revision number" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectNoteRevisionRef", description = Some "The stable identity of a project note revision." }

let ProjectNoteRevision =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectNoteRevisionRef" } s.reference.meta::{ description = Some "project note revision identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision title" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision description" }
            , body = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision Markdown body" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of the revision body are marked sensitive" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision creation timestamp" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal revision author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent revision author" }
            , author_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway revision author" }
            }
        }
        s.record.meta::{ name = Some "ProjectNoteRevision", description = Some "An immutable revision of a project note." }

let ProjectTaskRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "owning project identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project task identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectTaskRef", description = Some "The stable identity of a task attached to a project." }

let ProjectTask =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectTaskRef" } s.reference.meta::{ description = Some "project task identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task title" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of task content are marked sensitive" }
            , status = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task status: draft, ready, in_progress, done, or cancelled; defaults to draft" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project task is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task creation timestamp" }
            , updated_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task last update timestamp" }
            }
        , optional = toMap
            { description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "optional sensitive Markdown task description" }
            , creator_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal task creator" }
            , creator_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent task creator" }
            , creator_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway task creator" }
            , updater_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal last task updater" }
            , updater_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent last task updater" }
            , updater_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway last task updater" }
            }
        }
        s.record.meta::{ name = Some "ProjectTask", description = Some "A flat task attached to a project." }

let ProjectSecretRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "owning project identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project secret identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectSecretRef", description = Some "The stable identity of a secret attached to a project." }

let ProjectSecret =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectSecretRef" } s.reference.meta::{ description = Some "project secret identity" }
            , author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "immutable secret author" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "public secret description" }
            , ciphertext = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "encrypted secret value" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the project secret is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project secret creation timestamp" }
            , updated_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project secret update timestamp" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectSecret", description = Some "An encrypted secret attached to a project." }

let ProjectRecordSchemaRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { project = s.reference.from s.reference.props::{ to = "ProjectRef" } s.reference.meta::{ description = Some "owning project identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project record schema identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectRecordSchemaRef", description = Some "The stable identity of a project record schema." }

let ProjectRecordSchema =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectRecordSchemaRef" } s.reference.meta::{ description = Some "record schema identity" }
            , name = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "immutable schema machine name" }
            , label = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "schema display label" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "schema description" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "schema creation timestamp" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal responsible for the schema state" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent responsible for the schema state" }
            }
        }
        s.record.meta::{ name = Some "ProjectRecordSchema", description = Some "A project-local schema for structured records." }

let ProjectRecordAttributeRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { schema = s.reference.from s.reference.props::{ to = "ProjectRecordSchemaRef" } s.reference.meta::{ description = Some "owning record schema" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project record attribute identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectRecordAttributeRef", description = Some "The stable identity of a project record attribute." }

let ProjectRecordAttribute =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectRecordAttributeRef" } s.reference.meta::{ description = Some "record attribute identity" }
            , name = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "immutable attribute machine name" }
            , label = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "attribute display label" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "attribute description" }
            , type = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "attribute type" }
            , cardinality = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "one or many values" }
            , uniqueness = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "none, record, or global uniqueness" }
            , display = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "default display tier" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "attribute creation timestamp" }
            }
        , optional = toMap
            { target_schema = s.reference.from s.reference.props::{ to = "ProjectRecordSchemaRef" } s.reference.meta::{ description = Some "target schema for record references" }
            , author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal responsible for the attribute state" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent responsible for the attribute state" }
            }
        }
        s.record.meta::{ name = Some "ProjectRecordAttribute", description = Some "A typed field defined by a project record schema." }

let ProjectRecordRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { schema = s.reference.from s.reference.props::{ to = "ProjectRecordSchemaRef" } s.reference.meta::{ description = Some "owning record schema" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project record identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectRecordRef", description = Some "The stable identity of a project record." }

let ProjectRecord =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectRecordRef" } s.reference.meta::{ description = Some "record identity" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "record creation timestamp" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal record author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent record author" }
            }
        }
        s.record.meta::{ name = Some "ProjectRecord", description = Some "An immutable container for values in a project record schema." }

let ProjectRecordValueRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { record = s.reference.from s.reference.props::{ to = "ProjectRecordRef" } s.reference.meta::{ description = Some "owning record" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed project record value identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ProjectRecordValueRef", description = Some "The stable identity of a project record value." }

let ProjectRecordValue =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "ProjectRecordValueRef" } s.reference.meta::{ description = Some "record value identity" }
            , attribute = s.reference.from s.reference.props::{ to = "ProjectRecordAttributeRef" } s.reference.meta::{ description = Some "attribute that defines the value" }
            , value = s.any.from s.any.props::{ variant = s.any.variants.permissive } s.any.meta::{ description = Some "typed scalar value" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the value is sensitive" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "value creation timestamp" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal value author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent value author" }
            }
        }
        s.record.meta::{ name = Some "ProjectRecordValue", description = Some "An immutable typed value on a project record." }

let SessionNoteRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { session = s.reference.from s.reference.props::{ to = "SessionRef" } s.reference.meta::{ description = Some "owning session identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed session note identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionNoteRef", description = Some "The stable identity of a note attached to a session." }

let SessionNote =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SessionNoteRef" } s.reference.meta::{ description = Some "session note identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "note title" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "note description" }
            , body = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "Markdown note body" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of the current revision body are marked sensitive" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the session note is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session note creation timestamp" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "current immutable note revision" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal note author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent note author" }
            , author_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway note author" }
            }
        }
        s.record.meta::{ name = Some "SessionNote", description = Some "A shared Markdown note attached to a session." }

let SessionNoteRevisionRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { note = s.reference.from s.reference.props::{ to = "SessionNoteRef" } s.reference.meta::{ description = Some "revised session note identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "immutable revision number" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionNoteRevisionRef", description = Some "The stable identity of a session note revision." }

let SessionNoteRevision =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SessionNoteRevisionRef" } s.reference.meta::{ description = Some "session note revision identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision title" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision description" }
            , body = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision Markdown body" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of the revision body are marked sensitive" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "revision creation timestamp" }
            }
        , optional = toMap
            { author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal revision author" }
            , author_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent revision author" }
            , author_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway revision author" }
            }
        }
        s.record.meta::{ name = Some "SessionNoteRevision", description = Some "An immutable revision of a session note." }

let SessionTaskRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { session = s.reference.from s.reference.props::{ to = "SessionRef" } s.reference.meta::{ description = Some "owning session identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed session task identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionTaskRef", description = Some "The stable identity of a task attached to a session." }

let SessionTask =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SessionTaskRef" } s.reference.meta::{ description = Some "session task identity" }
            , title = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task title" }
            , sensitive = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether agent reads of task content are marked sensitive" }
            , status = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task status: draft, ready, in_progress, done, or cancelled; defaults to draft" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the session task is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task creation timestamp" }
            , updated_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "task last update timestamp" }
            }
        , optional = toMap
            { description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "optional sensitive Markdown task description" }
            , creator_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal task creator" }
            , creator_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent task creator" }
            , creator_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway task creator" }
            , updater_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "principal last task updater" }
            , updater_agent = s.reference.from s.reference.props::{ to = "WorkspaceAgentRef" } s.reference.meta::{ description = Some "workspace agent last task updater" }
            , updater_gateway = s.reference.from s.reference.props::{ to = "GatewayRef" } s.reference.meta::{ description = Some "gateway last task updater" }
            }
        }
        s.record.meta::{ name = Some "SessionTask", description = Some "A flat task attached to a session." }

let SessionSecretRef =
      s.record.from
        s.record.props::{
        , required = toMap
            { session = s.reference.from s.reference.props::{ to = "SessionRef" } s.reference.meta::{ description = Some "owning session identity" }
            , id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed session secret identity" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionSecretRef", description = Some "The stable identity of a secret attached to a session." }

let SessionSecret =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SessionSecretRef" } s.reference.meta::{ description = Some "session secret identity" }
            , author_principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "immutable secret author" }
            , description = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "public secret description" }
            , ciphertext = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "encrypted secret value" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the session secret is available" }
            , created_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session secret creation timestamp" }
            , updated_at = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session secret update timestamp" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SessionSecret", description = Some "An encrypted secret attached to a session." }

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

let SessionEventMetrics =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional =
            toMap
              { request_ms =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider request duration in milliseconds" }
              , input_tokens =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider-reported input token count" }
              , cached_input_tokens =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider-reported cached input token count" }
              , output_tokens =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider-reported output token count" }
              , reasoning_tokens =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider-reported reasoning token count" }
              , total_tokens =
                  s.number.from
                    s.number.props::{ variant = s.number.variants.integer }
                    s.number.meta::{ description = Some "provider-reported total token count" }
              }
        }
        s.record.meta::{ name = Some "SessionEventMetrics", description = Some "Optional provider metrics recorded with a session event." }

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
               , metrics =
                   s.reference.from
                     s.reference.props::{ to = "SessionEventMetrics" }
                     s.reference.meta::{ description = Some "optional event metrics" }
               }
        }
        s.record.meta::{ name = Some "SessionEvent", description = Some "An immutable event in a durable workspace session." }

let ActivityEventRef =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "durable typed activity identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "ActivityEventRef", description = Some "The stable identity of an activity event." }

let SystemGrantRef =
      s.record.from
        s.record.props::{
        , required = toMap { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "durable typed system grant identity" } }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SystemGrantRef", description = Some "The stable identity of a system manager grant." }

let SystemGrant =
      s.record.from
        s.record.props::{
        , required = toMap
            { ref = s.reference.from s.reference.props::{ to = "SystemGrantRef" } s.reference.meta::{ description = Some "system grant identity" }
            , principal = s.reference.from s.reference.props::{ to = "PrincipalRef" } s.reference.meta::{ description = Some "granted principal identity" }
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the system manager grant is enabled" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "monotonic system grant revision" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SystemGrant", description = Some "A principal system manager grant." }

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
                { resource_keychain_id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "keychain resource ID" }
				, resource_system_grant = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "system grant resource ID" }
               , resource_keychain_version = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "keychain resource version" }
               , resource_agent_provider = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent provider resource ID" }
               , resource_agent_model = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent model resource ID" }
               , resource_group = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "group resource ID" }
               , resource_group_member_group = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "group membership group ID" }
               , resource_group_member_principal = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "group membership principal ID" }
               , resource_identity = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "identity resource ID" }
               , resource_principal = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "principal resource ID" }
               , resource_project = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project resource ID" }
               , resource_project_file = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project file resource ID" }
               , resource_project_grant = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project grant resource ID" }
                , resource_project_note = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project note resource ID" }
				, resource_project_task = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project task resource ID" }
                , resource_project_secret = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project secret resource ID" }
                , resource_project_record_schema = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project record schema resource ID" }
                , resource_project_record_attribute = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project record attribute resource ID" }
                , resource_project_record = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "project record resource ID" }
               , resource_session = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session resource ID" }
               , resource_session_event = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session event resource ID" }
               , resource_session_file = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session file resource ID" }
               , resource_session_grant = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session grant resource ID" }
                , resource_session_note = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session note resource ID" }
				, resource_session_task = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session task resource ID" }
                , resource_session_secret = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "session secret resource ID" }
               , resource_storage_provider = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "storage provider resource ID" }
               , resource_workspace = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace resource ID" }
               , resource_workspace_agent_workspace = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace agent workspace ID" }
                , resource_workspace_agent_id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace agent binding ID" }
               , resource_workspace_grant = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace grant resource ID" }
               , resource_workspace_storage_provider_workspace = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace storage provider workspace ID" }
               , resource_workspace_storage_provider_provider = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "workspace storage provider provider ID" }
               }
        }
        s.record.meta::{ name = Some "ActivityEvent", description = Some "A durable global activity event with a typed resource target." }

let ActivityCursor =
      s.record.from
        s.record.props::{
        , required =
            toMap
               { id =
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
               , events =
                   s.list.from
                     s.list.props::{
                     , values =
                         s.text.from
                           s.text.props::{ variant = s.text.variants.none }
                           s.text.meta::{ description = Some "exact activity event names or terminal wildcard selectors" }
                     }
                     s.list.meta::{ description = Some "activity event selectors" }
               }
         , optional =
             toMap
               { name =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                     s.text.meta::{ description = Some "opaque client checkpoint correlation name" }
               , cursor =
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
            , s.root.from GroupRef s.root.meta::{ name = "GroupRef" }
            , s.root.from Group s.root.meta::{ name = "Group" }
           , s.root.from GroupMemberRef s.root.meta::{ name = "GroupMemberRef" }
           , s.root.from GroupMember s.root.meta::{ name = "GroupMember" }
          , s.root.from ProjectRef s.root.meta::{ name = "ProjectRef" }
          , s.root.from Project s.root.meta::{ name = "Project" }
          , s.root.from ProjectPrincipalGrant s.root.meta::{ name = "ProjectPrincipalGrant" }
          , s.root.from ProjectGroupGrant s.root.meta::{ name = "ProjectGroupGrant" }
            , s.root.from SessionRef s.root.meta::{ name = "SessionRef" }
           , s.root.from Session s.root.meta::{ name = "Session" }
			, s.root.from SystemGrantRef s.root.meta::{ name = "SystemGrantRef" }
			, s.root.from SystemGrant s.root.meta::{ name = "SystemGrant" }
            , s.root.from StorageObjectRef s.root.meta::{ name = "StorageObjectRef" }
           , s.root.from StorageObject s.root.meta::{ name = "StorageObject" }
           , s.root.from SessionFileRef s.root.meta::{ name = "SessionFileRef" }
           , s.root.from SessionFile s.root.meta::{ name = "SessionFile" }
            , s.root.from ProjectFileRef s.root.meta::{ name = "ProjectFileRef" }
            , s.root.from ProjectFile s.root.meta::{ name = "ProjectFile" }
			, s.root.from ProjectNoteRef s.root.meta::{ name = "ProjectNoteRef" }
			, s.root.from ProjectNote s.root.meta::{ name = "ProjectNote" }
			, s.root.from ProjectNoteRevisionRef s.root.meta::{ name = "ProjectNoteRevisionRef" }
			, s.root.from ProjectNoteRevision s.root.meta::{ name = "ProjectNoteRevision" }
			, s.root.from ProjectTaskRef s.root.meta::{ name = "ProjectTaskRef" }
			, s.root.from ProjectTask s.root.meta::{ name = "ProjectTask" }
			, s.root.from ProjectSecretRef s.root.meta::{ name = "ProjectSecretRef" }
			, s.root.from ProjectSecret s.root.meta::{ name = "ProjectSecret" }
			, s.root.from ProjectRecordSchemaRef s.root.meta::{ name = "ProjectRecordSchemaRef" }
			, s.root.from ProjectRecordSchema s.root.meta::{ name = "ProjectRecordSchema" }
			, s.root.from ProjectRecordAttributeRef s.root.meta::{ name = "ProjectRecordAttributeRef" }
			, s.root.from ProjectRecordAttribute s.root.meta::{ name = "ProjectRecordAttribute" }
			, s.root.from ProjectRecordRef s.root.meta::{ name = "ProjectRecordRef" }
			, s.root.from ProjectRecord s.root.meta::{ name = "ProjectRecord" }
			, s.root.from ProjectRecordValueRef s.root.meta::{ name = "ProjectRecordValueRef" }
			, s.root.from ProjectRecordValue s.root.meta::{ name = "ProjectRecordValue" }
              , s.root.from SessionNoteRef s.root.meta::{ name = "SessionNoteRef" }
              , s.root.from SessionNote s.root.meta::{ name = "SessionNote" }
			 , s.root.from SessionNoteRevisionRef s.root.meta::{ name = "SessionNoteRevisionRef" }
			 , s.root.from SessionNoteRevision s.root.meta::{ name = "SessionNoteRevision" }
			 , s.root.from SessionTaskRef s.root.meta::{ name = "SessionTaskRef" }
			 , s.root.from SessionTask s.root.meta::{ name = "SessionTask" }
			 , s.root.from SessionSecretRef s.root.meta::{ name = "SessionSecretRef" }
			 , s.root.from SessionSecret s.root.meta::{ name = "SessionSecret" }
             , s.root.from GatewayRef s.root.meta::{ name = "GatewayRef" }
             , s.root.from SessionEventRef s.root.meta::{ name = "SessionEventRef" }
             , s.root.from SessionEventMetrics s.root.meta::{ name = "SessionEventMetrics" }
             , s.root.from SessionEvent s.root.meta::{ name = "SessionEvent" }
          , s.root.from ActivityEventRef s.root.meta::{ name = "ActivityEventRef" }
          , s.root.from ActivityEvent s.root.meta::{ name = "ActivityEvent" }
          , s.root.from ActivityCursor s.root.meta::{ name = "ActivityCursor" }
          , s.root.from ActivityTopicCheckpoint s.root.meta::{ name = "ActivityTopicCheckpoint" }
          , s.root.from ActivityTopicCheckpoints s.root.meta::{ name = "ActivityTopicCheckpoints" }
          , s.root.from SessionPrincipalGrant s.root.meta::{ name = "SessionPrincipalGrant" }
         , s.root.from SessionGroupGrant s.root.meta::{ name = "SessionGroupGrant" }
        ]
    }
