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

let Workspace =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { key =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable workspace key" }
              , name =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "workspace display name" }
              }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the workspace is enabled" }
              }
        }
        s.record.meta::{ name = Some "Workspace" }

let Workspaces =
      s.list.from
        s.list.props::{ values = Workspace }
        s.list.meta::{ description = Some "configured workspaces" }

let GatehouseConfig =
      s.record.from
        s.record.props::{
        , required = toMap
            { api_version =
                s.text.from
                  s.text.props::{ variant = s.text.variants.literal "v1" }
                  s.text.meta::{ description = Some "configuration API version" }
              }
        , optional = toMap { database = Database, workspaces = Workspaces }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
