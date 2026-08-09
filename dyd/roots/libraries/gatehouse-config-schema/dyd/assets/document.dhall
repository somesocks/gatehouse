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

let GroupToolGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { tool =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "granted workspace-local tool identity" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the tool grant is enabled" }
              }
        }
        s.record.meta::{ name = Some "GroupToolGrant" }

let GroupToolGrants =
      s.list.from
        s.list.props::{ values = GroupToolGrant }
        s.list.meta::{ description = Some "configured direct tool grants" }

let GroupResourceGrant =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { resource =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "granted workspace-local resource identity" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource grant is enabled" }
              }
        }
        s.record.meta::{ name = Some "GroupResourceGrant" }

let GroupResourceGrants =
      s.list.from
        s.list.props::{ values = GroupResourceGrant }
        s.list.meta::{ description = Some "configured direct resource grants" }

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
               , tool_grants = GroupToolGrants
               , resource_grants = GroupResourceGrants
               }
        }
        s.record.meta::{ name = Some "Group" }

let Groups =
      s.list.from
        s.list.props::{ values = Group }
        s.list.meta::{ description = Some "configured workspace authorization groups" }

let Tool =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local tool identity" }
              , ref =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "tool module reference" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the tool is enabled" }
              }
        }
        s.record.meta::{ name = Some "Tool" }

let Tools =
      s.list.from
        s.list.props::{ values = Tool }
        s.list.meta::{ description = Some "configured workspace tools" }

let Resource =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace-local resource identity" }
               , source =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                     s.text.meta::{ description = Some "resource value source" }
              , secret =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource value is secret" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the resource is enabled" }
              }
        }
        s.record.meta::{ name = Some "Resource" }

let Resources =
      s.list.from
        s.list.props::{ values = Resource }
        s.list.meta::{ description = Some "configured workspace resources" }

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
               , tools = Tools
               , resources = Resources
               }
        }
        s.record.meta::{ name = Some "Workspace" }

let Workspaces =
      s.list.from
        s.list.props::{ values = Workspace }
        s.list.meta::{ description = Some "configured workspaces" }

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
              , verifiers = Verifiers
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
              }
        }
        s.record.meta::{ name = Some "Identity" }

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

let KeychainPassphraseSource =
      s.text.from
        s.text.props::{ variant = s.text.variants.none }
        s.text.meta::{ description = Some "keychain passphrase source" }

let KeychainPassphraseSources =
      s.list.from
        s.list.props::{ values = KeychainPassphraseSource }
        s.list.meta::{ description = Some "ordered keychain passphrase sources" }

let Keychain =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { id =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable keychain identity" }
               , sources = KeychainPassphraseSources
              }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "Keychain" }

let Keychains =
      s.list.from
        s.list.props::{ values = Keychain }
        s.list.meta::{ description = Some "configured keychain passphrase sources" }

let GatehouseConfig =
      s.record.from
        s.record.props::{
        , required = toMap
            { api_version =
                s.text.from
                  s.text.props::{ variant = s.text.variants.literal "v1" }
                  s.text.meta::{ description = Some "configuration API version" }
              }
        , optional = toMap
            { database = Database
            , workspaces = Workspaces
            , principals = Principals
            , keychains = Keychains
            }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
