let Grammar = ./dhall-codegen/grammar.dhall

let s = Grammar.Schema

let text =
      s.text.from
        s.text.props::{ variant = s.text.variants.none }
        s.text.meta::{=}

let integer =
      s.number.from
        s.number.props::{ variant = s.number.variants.integer }
        s.number.meta::{=}

let boolean =
      s.boolean.from
        s.boolean.props::{=}
        s.boolean.meta::{=}

let any =
      s.any.from
        s.any.props::{ variant = s.any.variants.permissive }
        s.any.meta::{=}

let PullPolicy =
      s.oneOf.from
        { options =
            [ s.text.from
                s.text.props::{ variant = s.text.variants.literal "Always" }
                s.text.meta::{=}
            , s.text.from
                s.text.props::{ variant = s.text.variants.literal "IfNotPresent" }
                s.text.meta::{=}
            , s.text.from
                s.text.props::{ variant = s.text.variants.literal "Never" }
                s.text.meta::{=}
            ]
        }
        s.oneOf.meta::{=}

let StrategyType =
      s.oneOf.from
        { options =
            [ s.text.from
                s.text.props::{ variant = s.text.variants.literal "Recreate" }
                s.text.meta::{=}
            , s.text.from
                s.text.props::{ variant = s.text.variants.literal "RollingUpdate" }
                s.text.meta::{=}
            ]
        }
        s.oneOf.meta::{=}

let Image =
      s.record.from
        s.record.props::{
        , required = toMap { repository = text }
        , optional = toMap { tag = text, pullPolicy = PullPolicy }
        }
        s.record.meta::{=}

let ImagePullSecrets =
      s.list.from
        s.list.props::{
        , values =
            s.record.from
              s.record.props::{ required = toMap { name = text } }
              s.record.meta::{=}
        }
        s.list.meta::{=}

let Service =
      s.record.from
        s.record.props::{
        , required = toMap { port = integer }
        , optional = toMap { annotations = any }
        }
        s.record.meta::{=}

let DeploymentStrategy =
      s.record.from
        s.record.props::{
        , required = toMap { type = StrategyType }
        , optional = toMap { rollingUpdate = any }
        }
        s.record.meta::{=}

let Environment =
      s.list.from
        s.list.props::{
        , values =
            s.record.from
              s.record.props::{ required = toMap { name = text, value = text } }
              s.record.meta::{=}
        }
        s.list.meta::{=}

let Volumes =
      s.list.from
        s.list.props::{
        , values =
            s.record.from
              s.record.props::{
              , required = toMap { name = text, mountPath = text, source = any }
              , optional = toMap { readOnly = boolean, subPath = text }
              }
              s.record.meta::{=}
        }
        s.list.meta::{=}

let HelmValues =
      s.record.from
        s.record.props::{
        , required =
            toMap
              { config = text
              , image = Image
              , replicaCount = integer
              , service = Service
              , env = Environment
              , volumes = Volumes
              , deploymentStrategy = DeploymentStrategy
              }
        , optional =
            toMap
              { nameOverride = text
              , fullnameOverride = text
              , imagePullSecrets = ImagePullSecrets
              , resources = any
              , podAnnotations = any
              , podLabels = any
              , podSecurityContext = any
              , containerSecurityContext = any
              , nodeSelector = any
              , tolerations = s.list.from s.list.props::{ values = any } s.list.meta::{=}
              , affinity = any
              }
        }
        s.record.meta::{ description = Some "Gatehouse Helm chart values" }

let HelmValuesRoot =
      s.root.from HelmValues s.root.meta::{ name = "GatehouseHelmValues" }

in  Grammar.Document::{ schemas = [ HelmValuesRoot ] }
