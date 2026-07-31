let Grammar = ./dhall-codegen/grammar.dhall

let Document = Grammar.Document

let s = Grammar.Schema

let SQLiteDatabase =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.literal "sqlite" }
                    s.text.meta::{ description = Some "persistent SQLite database" }
              , path =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "SQLite database path" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "SQLiteDatabase" }

let PostgresDatabase =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.literal "postgres" }
                    s.text.meta::{ description = Some "persistent PostgreSQL database" }
              , url =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "PostgreSQL URL reference" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "PostgresDatabase" }

let EphemeralDatabase =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.literal "ephemeral" }
                    s.text.meta::{ description = Some "non-persistent database" }
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "EphemeralDatabase" }

let Database =
      s.oneOf.from
        { options = [ SQLiteDatabase, PostgresDatabase, EphemeralDatabase ] }
        s.oneOf.meta::{ name = Some "Database" }

let GroupMember =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { principal =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "assigned principal identity" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the group assignment is enabled" }
              }
        }
        s.record.meta::{ name = Some "GroupMember" }

let GroupMembers =
      s.list.from
        s.list.props::{ values = GroupMember }
        s.list.meta::{ description = Some "configured group members" }

let Group =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local group identity" }
              }
        , optional =
            toMap
              { name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "group display name" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the group is enabled" }
              , members = GroupMembers
              }
        }
        s.record.meta::{ name = Some "Group" }

let Groups =
      s.list.from
        s.list.props::{ values = Group }
        s.list.meta::{ description = Some "configured workspace authorization groups" }

let Workspace =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable workspace identity" }
              }
        , optional =
            toMap
              { name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace display name" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the workspace is enabled" }
              , groups = Groups
              }
        }
        s.record.meta::{ name = Some "Workspace" }

let Workspaces =
      s.list.from
        s.list.props::{ values = Workspace }
        s.list.meta::{ description = Some "configured workspaces" }

let MatrixIdentity =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.literal "matrix" }
                    s.text.meta::{ description = Some "Matrix identity kind" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "canonical Matrix user identifier" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        }
        s.record.meta::{ name = Some "MatrixIdentity" }

let GatehouseIdentity =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { kind =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.literal "gatehouse" }
                    s.text.meta::{ description = Some "Gatehouse identity kind" }
              , id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "Gatehouse user name" }
              , password_verifier =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "Argon2id password verifier or environment reference" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        }
        s.record.meta::{ name = Some "GatehouseIdentity" }

let Identity =
      s.oneOf.from
        { options = [ MatrixIdentity, GatehouseIdentity ] }
        s.oneOf.meta::{ name = Some "Identity" }

let Identities =
      s.list.from
        s.list.props::{ values = Identity }
        s.list.meta::{ description = Some "configured principal identities" }

let Principal =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable principal identity" }
              }
        , optional =
            toMap
              { name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "principal display name" }
              , enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the principal is enabled" }
              , identities = Identities
              }
        }
        s.record.meta::{ name = Some "Principal" }

let Principals =
      s.list.from
        s.list.props::{ values = Principal }
        s.list.meta::{ description = Some "configured principals" }

let GatehouseConfig =
      s.record.from
        s.record.props::{
        , required = toMap
            { api_version =
                s.text.from
                  s.text.props::{ variant = s.text.variants.literal "v1" }
                  s.text.meta::{ description = Some "configuration API version" }
              }
        , optional = toMap { database = Database, workspaces = Workspaces, principals = Principals }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
