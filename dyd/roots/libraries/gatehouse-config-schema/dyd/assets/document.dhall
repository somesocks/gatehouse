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

let GatehouseConfig =
      s.record.from
        s.record.props::{
        , required = toMap
            { api_version =
                s.text.from
                  s.text.props::{ variant = s.text.variants.literal "v1" }
                  s.text.meta::{ description = Some "configuration API version" }
              }
        , optional = toMap { database = Database }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
