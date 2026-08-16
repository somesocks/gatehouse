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
               , source =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                     s.text.meta::{ description = Some "tool module source" }
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

let WorkspaceAgent =
      s.record.from
        s.record.props::{
        , required = toMap
            { model = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent model identity" }
            , priority = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "agent selection priority" }
            }
        , optional = toMap
             { enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the agent is enabled" }
             , label = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent display label" }
             , max_turns = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "maximum tool-using model turns per reply" }
             , system_prompt = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "complete system prompt for the agent" }
             }
        }
        s.record.meta::{ name = Some "WorkspaceAgent" }

let WorkspaceAgents =
      s.list.from s.list.props::{ values = WorkspaceAgent }
        s.list.meta::{ description = Some "configured workspace agents" }

let WorkspaceStorageProvider =
      s.record.from
        s.record.props::{
        , required = toMap
            { provider = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "storage provider identity" }
            , priority = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "new-object placement priority" }
            }
        , optional = toMap
            { enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the workspace may place new objects with this provider" } }
        }
        s.record.meta::{ name = Some "WorkspaceStorageProvider" }

let WorkspaceStorageProviders =
      s.list.from s.list.props::{ values = WorkspaceStorageProvider }
        s.list.meta::{ description = Some "storage providers available for new workspace objects" }

let Workspace =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable workspace reconciliation alias" }
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
                 , agents = WorkspaceAgents
                 , storage_providers = WorkspaceStorageProviders
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
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable identity reconciliation alias" }
              , key =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "globally namespaced login identity" }
              , verifiers = Verifiers
              }
        , optional =
            toMap
               { enabled =
                   s.boolean.from
                     s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the identity is enabled" }
               , revision =
                   s.number.from
                     s.number.props::{ variant = s.number.variants.integer }
                     s.number.meta::{ description = Some "monotonic identity configuration revision" }
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
              { alias =
                  s.text.from
                    s.text.props::{ variant = s.text.variants.none }
                    s.text.meta::{ description = Some "stable principal reconciliation alias" }
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

let AgentProviderAPIKey =
      s.record.from
        s.record.props::{
        , required = toMap
            { sources = s.list.from
                s.list.props::{ values = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "API key source" } }
                s.list.meta::{ description = Some "ordered API key sources" }
            }
        , optional = toMap
            { keychain = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "keychain used to seal the API key" } }
        }
        s.record.meta::{ name = Some "AgentProviderAPIKey" }

let AgentProvider =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "stable agent provider identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "monotonic provider configuration revision" }
            , protocol = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider protocol" }
            }
        , optional =
            toMap
              { base_url = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider base URL" }
              , api_key = AgentProviderAPIKey
              , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the provider is enabled" }
              }
        }
        s.record.meta::{ name = Some "AgentProvider" }

let AgentProviders =
      s.list.from s.list.props::{ values = AgentProvider }
        s.list.meta::{ description = Some "configured agent providers" }

let AgentModel =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "stable agent model identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "monotonic model configuration revision" }
            , provider = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "agent provider identity" }
            , model = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "provider model identifier" }
            , parameters = s.any.from s.any.props::{ variant = s.any.variants.permissive } s.any.meta::{ description = Some "provider model parameters" }
            }
        , optional = toMap
            { enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the model is enabled" } }
        }
        s.record.meta::{ name = Some "AgentModel" }

let AgentModels =
      s.list.from s.list.props::{ values = AgentModel }
        s.list.meta::{ description = Some "configured agent models" }

let StorageProviderSecretAccessKey =
      s.record.from
        s.record.props::{
        , required = toMap
            { sources = s.list.from
                s.list.props::{ values = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "secret access key source" } }
                s.list.meta::{ description = Some "ordered secret access key sources" }
            }
        , optional = toMap
            { keychain = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "keychain used to seal the secret access key" } }
        }
        s.record.meta::{ name = Some "StorageProviderSecretAccessKey" }

let StorageProviderS3Credentials =
      s.record.from
        s.record.props::{
        , required = toMap
            { access_key_id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "S3 access key ID" }
            , secret_access_key = StorageProviderSecretAccessKey
            }
        , optional = [] : List { mapKey : Text, mapValue : s.type }
        }
        s.record.meta::{ name = Some "StorageProviderS3Credentials" }

let StorageProvider =
      s.record.from
        s.record.props::{
        , required = toMap
            { id = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "stable storage provider identity" }
            , revision = s.number.from s.number.props::{ variant = s.number.variants.integer } s.number.meta::{ description = Some "monotonic provider configuration revision" }
            , protocol = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "storage provider protocol" }
            }
        , optional = toMap
            { endpoint = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "S3-compatible endpoint URL" }
            , region = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "S3 region" }
            , bucket = s.text.from s.text.props::{ variant = s.text.variants.none } s.text.meta::{ description = Some "S3 bucket" }
            , credentials = StorageProviderS3Credentials
            , enabled = s.boolean.from s.boolean.props::{=} s.boolean.meta::{ description = Some "whether the provider is enabled" }
            }
        }
        s.record.meta::{ name = Some "StorageProvider" }

let StorageProviders =
      s.list.from s.list.props::{ values = StorageProvider }
        s.list.meta::{ description = Some "configured storage providers" }

let HTTPComponent =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the HTTP route group is enabled" }
              }
        }
        s.record.meta::{ name = Some "HTTPComponent" }

let HTTPService =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional =
            toMap
              { enabled =
                  s.boolean.from
                    s.boolean.props::{=}
                    s.boolean.meta::{ description = Some "whether the HTTP service is enabled" }
               , listen =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                     s.text.meta::{ description = Some "TCP listener address" }
               , keychain =
                   s.text.from
                     s.text.props::{ variant = s.text.variants.none }
                     s.text.meta::{ description = Some "keychain used for sealed bearer tokens" }
               , web = HTTPComponent
               , api = HTTPComponent
              }
        }
        s.record.meta::{ name = Some "HTTPService" }

let Services =
      s.record.from
        s.record.props::{
        , required = [] : List { mapKey : Text, mapValue : s.type }
        , optional = toMap { http = HTTPService }
        }
        s.record.meta::{ name = Some "Services" }

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
               , agent_providers = AgentProviders
               , agent_models = AgentModels
               , storage_providers = StorageProviders
               , services = Services
             }
        }
        s.record.meta::{ description = Some "Gatehouse configuration" }

let GatehouseConfig =
      s.root.from GatehouseConfig s.root.meta::{ name = "GatehouseConfig" }

in  Document::{ headers = [] : List Text, schemas = [ GatehouseConfig ] }
