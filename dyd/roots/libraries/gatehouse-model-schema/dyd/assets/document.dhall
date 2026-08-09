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
              { name =
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
                     s.text.meta::{ description = Some "stable principal identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "PrincipalRef"
        , description = Some "The stable identity of a principal."
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
              { name =
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
                    s.text.meta::{ description = Some "globally namespaced identity" }
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
        , optional = [] : List { mapKey : Text, mapValue : s.type }
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
                     s.text.meta::{ description = Some "workspace-local group identity" }
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
              { name =
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
                    s.text.meta::{ description = Some "workspace-local session identity" }
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
              , created_by =
                  s.reference.from
                    s.reference.props::{ to = "PrincipalRef" }
                    s.reference.meta::{ description = Some "creating principal identity" }
              , created_at =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "session creation timestamp" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the session is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "Session"
        , description = Some "A durable workspace session."
        }

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
                     s.text.meta::{ description = Some "workspace-local tool identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
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
                     s.text.meta::{ description = Some "workspace-local resource identity" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
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
          , s.root.from ResourceRef s.root.meta::{ name = "ResourceRef" }
          , s.root.from ToolRef s.root.meta::{ name = "ToolRef" }
           , s.root.from GroupRef s.root.meta::{ name = "GroupRef" }
           , s.root.from Group s.root.meta::{ name = "Group" }
         , s.root.from GroupMember s.root.meta::{ name = "GroupMember" }
         , s.root.from SessionRef s.root.meta::{ name = "SessionRef" }
         , s.root.from Session s.root.meta::{ name = "Session" }
         , s.root.from SessionPrincipalGrant s.root.meta::{ name = "SessionPrincipalGrant" }
         , s.root.from SessionGroupGrant s.root.meta::{ name = "SessionGroupGrant" }
         , s.root.from Tool s.root.meta::{ name = "Tool" }
        , s.root.from Resource s.root.meta::{ name = "Resource" }
        , s.root.from GroupToolGrant s.root.meta::{ name = "GroupToolGrant" }
        , s.root.from GroupResourceGrant s.root.meta::{ name = "GroupResourceGrant" }
        ]
    }
