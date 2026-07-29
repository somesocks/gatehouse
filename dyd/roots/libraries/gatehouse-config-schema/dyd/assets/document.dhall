let Grammar = ./dhall-codegen/grammar.dhall

let Document = Grammar.Document

let s = Grammar.Schema

let GatehouseConfig =
      s.record.from
        s.record.props::{
        , required = toMap
            { api_version =
                s.text.from
                  s.text.props::{ variant = s.text.variants.literal "v1" }
                  s.text.meta::{ description = Some "configuration API version" }
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
