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
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning principal identity" }
              , verifiers = Verifiers
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{
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

let Tool =
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
                    s.text.meta::{ description = Some "workspace-local tool identity" }
              , ref =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "tool module reference" }
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

let Resource =
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
                    s.text.meta::{ description = Some "workspace-local resource identity" }
              , ref =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "resource value reference" }
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
              { workspace =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning workspace identity" }
              , group =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local group identity" }
              , tool =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "granted workspace-local tool identity" }
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
              { workspace =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "owning workspace identity" }
              , group =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local group identity" }
              , resource =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "granted workspace-local resource identity" }
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
        [ s.root.from Workspace s.root.meta::{ name = "Workspace" }
        , s.root.from Principal s.root.meta::{ name = "Principal" }
         , s.root.from Identity s.root.meta::{ name = "Identity" }
         , s.root.from Group s.root.meta::{ name = "Group" }
        , s.root.from GroupMember s.root.meta::{ name = "GroupMember" }
        , s.root.from Tool s.root.meta::{ name = "Tool" }
        , s.root.from Resource s.root.meta::{ name = "Resource" }
        , s.root.from GroupToolGrant s.root.meta::{ name = "GroupToolGrant" }
        , s.root.from GroupResourceGrant s.root.meta::{ name = "GroupResourceGrant" }
        ]
    }
