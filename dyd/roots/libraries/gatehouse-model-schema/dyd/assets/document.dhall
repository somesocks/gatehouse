let Grammar = ./dhall-codegen/grammar.dhall

let Document = Grammar.Document

let s = Grammar.Schema

let Workspace =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable workspace identity" }
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

let Principal =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable principal identity" }
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

let MatrixIdentity =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { matrix_user_id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "canonical Matrix user identifier" }
              , principal =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning principal identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "MatrixIdentity"
        , description = Some "A verified Matrix user identity."
        }

let GatehouseUserIdentity =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { username =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "Gatehouse user name" }
              , password_verifier =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "Argon2id password verifier" }
              , principal =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning principal identity" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
        , name = Some "GatehouseUserIdentity"
        , description = Some "A Gatehouse user identity authenticated by a password verifier."
        }

let Identity =
      s.oneOf.from
        { options =
            [ s.reference.from
                s.reference.props::{ to = "MatrixIdentity" }
                s.reference.meta::{ name = Some "MatrixIdentity" }
            , s.reference.from
                s.reference.props::{ to = "GatehouseUserIdentity" }
                s.reference.meta::{ name = Some "GatehouseUserIdentity" }
            ]
        }
        s.oneOf.meta::{
        , name = Some "Identity"
        , description = Some "A verified authentication binding for a principal."
        }

let Group =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { workspace =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning workspace identity" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local group identity" }
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
              { workspace =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning workspace identity" }
              , group =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local group identity" }
              , principal =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "assigned principal identity" }
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

in  Document::{
    , headers = [] : List Text
    , schemas =
        [ s.root.from Workspace s.root.meta::{ name = "Workspace" }
        , s.root.from Principal s.root.meta::{ name = "Principal" }
        , s.root.from MatrixIdentity s.root.meta::{ name = "MatrixIdentity" }
        , s.root.from GatehouseUserIdentity s.root.meta::{ name = "GatehouseUserIdentity" }
        , s.root.from Identity s.root.meta::{ name = "Identity" }
        , s.root.from Group s.root.meta::{ name = "Group" }
        , s.root.from GroupMember s.root.meta::{ name = "GroupMember" }
        ]
    }
