module Concourse exposing
  ( Build
  , BuildId
  , JobBuildIdentifier
  , BuildDuration
  , decodeBuild

  , BuildPrep
  , BuildPrepStatus(..)
  , decodeBuildPrep

  , BuildResources
  , BuildResourcesInput
  , BuildResourcesOutput
  , decodeBuildResources

  , BuildStatus(..)
  , decodeBuildStatus

  , Job
  , JobIdentifier
  , JobInput
  , JobOutput
  , decodeJob

  , Pipeline
  , PipelineIdentifier
  , PipelineGroup
  , decodePipeline

  , Metadata
  , MetadataField
  , decodeMetadata

  , Team
  , decodeTeam

  , User
  , decodeUser

  , Version
  , decodeVersion
  )

import Date exposing (Date)
import Dict exposing (Dict)
import Json.Decode exposing ((:=))
import Json.Decode.Extra exposing ((|:))

type alias Build =
  { id : BuildId
  , url : String
  , name : String
  , job : Maybe JobIdentifier
  , status : BuildStatus
  , duration : BuildDuration
  , reapTime : Maybe Date
  }

type alias BuildDuration =
  { startedAt : Maybe Date
  , finishedAt : Maybe Date
  }

type alias BuildId =
  Int

type alias BuildPrep =
  { pausedPipeline : BuildPrepStatus
  , pausedJob : BuildPrepStatus
  , maxRunningBuilds : BuildPrepStatus
  , inputs : Dict String BuildPrepStatus
  , inputsSatisfied : BuildPrepStatus
  , missingInputReasons : Dict String String
  }

type BuildPrepStatus
  = BuildPrepStatusUnknown
  | BuildPrepStatusBlocking
  | BuildPrepStatusNotBlocking

type alias BuildResources =
  { inputs : List BuildResourcesInput
  , outputs : List BuildResourcesOutput
  }

type alias BuildResourcesInput =
  { name : String
  , resource : String
  , type' : String
  , version : Version
  , metadata : Metadata
  , firstOccurrence : Bool
  }

type alias BuildResourcesOutput =
  { resource : String
  , version : Version
  }

type BuildStatus
  = BuildStatusPending
  | BuildStatusStarted
  | BuildStatusSucceeded
  | BuildStatusFailed
  | BuildStatusErrored
  | BuildStatusAborted

type alias JobBuildIdentifier =
  { teamName : String
  , pipelineName : String
  , jobName : String
  , buildName : String
  }

type alias JobIdentifier =
  { teamName : String
  , pipelineName : String
  , jobName : String
  }

type alias Job =
  { teamName : String
  , pipelineName : String
  , name : String
  , url : String
  , nextBuild : Maybe Build
  , finishedBuild : Maybe Build
  , paused : Bool
  , disableManualTrigger : Bool
  , inputs : List JobInput
  , outputs : List JobOutput
  , groups : List String
  }

type alias JobInput =
  { name : String
  , resource : String
  , passed : List String
  , trigger : Bool
  }

type alias JobOutput =
  { name : String
  , resource : String
  }

type alias PipelineIdentifier =
  { teamName : String
  , pipelineName : String
  }

type alias Pipeline =
  { name : String
  , url : String
  , paused : Bool
  , public : Bool
  , teamName : String
  , groups : List PipelineGroup
  }

type alias PipelineGroup =
  { name : String
  , jobs : List String
  , resources : List String
  }

type alias Metadata =
  List MetadataField

type alias MetadataField =
  { name : String
  , value : String
  }

type alias Team =
  { id : Int
  , name : String
  }

type alias User =
  { team : Team
  }

type alias Version =
  Dict String String

decodeBuild : Json.Decode.Decoder Build
decodeBuild =
  Json.Decode.object7 Build
    ("id" := Json.Decode.int)
    ("url" := Json.Decode.string)
    ("name" := Json.Decode.string)
    (Json.Decode.maybe (Json.Decode.object3 JobIdentifier
      ("job_name" := Json.Decode.string)
      ("team_name" := Json.Decode.string)
      ("pipeline_name" := Json.Decode.string)))
    ("status" := decodeBuildStatus)
    (Json.Decode.object2 BuildDuration
      (Json.Decode.maybe ("start_time" := (Json.Decode.map dateFromSeconds Json.Decode.float)))
      (Json.Decode.maybe ("end_time" := (Json.Decode.map dateFromSeconds Json.Decode.float))))
    (Json.Decode.maybe ("reap_time" := (Json.Decode.map dateFromSeconds Json.Decode.float)))

decodeBuildPrep : Json.Decode.Decoder BuildPrep
decodeBuildPrep =
  Json.Decode.succeed BuildPrep
    |: ("paused_pipeline" := decodeBuildPrepStatus)
    |: ("paused_job" := decodeBuildPrepStatus)
    |: ("max_running_builds" := decodeBuildPrepStatus)
    |: ("inputs" := Json.Decode.dict decodeBuildPrepStatus)
    |: ("inputs_satisfied" := decodeBuildPrepStatus)
    |: (defaultTo Dict.empty <| "missing_input_reasons" := Json.Decode.dict Json.Decode.string)

decodeBuildPrepStatus : Json.Decode.Decoder BuildPrepStatus
decodeBuildPrepStatus =
  Json.Decode.customDecoder Json.Decode.string <| \status ->
    case status of
      "unknown" ->
        Ok BuildPrepStatusUnknown
      "blocking" ->
        Ok BuildPrepStatusBlocking
      "not_blocking" ->
        Ok BuildPrepStatusNotBlocking
      unknown ->
        Err ("unknown build preparation status: " ++ unknown)

decodeBuildResources : Json.Decode.Decoder BuildResources
decodeBuildResources =
  Json.Decode.succeed BuildResources
    |: ("inputs" := Json.Decode.list decodeResourcesInput)
    |: ("outputs" := Json.Decode.list decodeResourcesOutput)


decodeResourcesInput : Json.Decode.Decoder BuildResourcesInput
decodeResourcesInput =
  Json.Decode.succeed BuildResourcesInput
    |: ("name" := Json.Decode.string)
    |: ("resource" := Json.Decode.string)
    |: ("type" := Json.Decode.string)
    |: ("version" := decodeVersion)
    |: ("metadata" := decodeMetadata)
    |: ("first_occurrence" := Json.Decode.bool)

decodeResourcesOutput : Json.Decode.Decoder BuildResourcesOutput
decodeResourcesOutput =
  Json.Decode.succeed BuildResourcesOutput
    |: ("resource" := Json.Decode.string)
    |: ("version" := Json.Decode.dict Json.Decode.string)

decodeBuildStatus : Json.Decode.Decoder BuildStatus
decodeBuildStatus =
  Json.Decode.customDecoder Json.Decode.string <| \status ->
    case status of
      "pending" ->
        Ok BuildStatusPending
      "started" ->
        Ok BuildStatusStarted
      "succeeded" ->
        Ok BuildStatusSucceeded
      "failed" ->
        Ok BuildStatusFailed
      "errored" ->
        Ok BuildStatusErrored
      "aborted" ->
        Ok BuildStatusAborted
      unknown ->
        Err ("unknown build status: " ++ unknown)

decodeJob : String -> String -> Json.Decode.Decoder Job
decodeJob teamName pipelineName =
  Json.Decode.succeed (Job teamName pipelineName)
    |: ("name" := Json.Decode.string)
    |: ("url" := Json.Decode.string)
    |: (Json.Decode.maybe ("next_build" := decodeBuild))
    |: (Json.Decode.maybe ("finished_build" := decodeBuild))
    |: (defaultTo False <| "paused" := Json.Decode.bool)
    |: (defaultTo False <| "disable_manual_trigger" := Json.Decode.bool)
    |: (defaultTo [] <| "inputs" := Json.Decode.list decodeJobInput)
    |: (defaultTo [] <| "outputs" := Json.Decode.list decodeJobOutput)
    |: (defaultTo [] <| "groups" := Json.Decode.list Json.Decode.string)

decodeJobInput : Json.Decode.Decoder JobInput
decodeJobInput =
  Json.Decode.succeed JobInput
    |: ("name" := Json.Decode.string)
    |: ("resource" := Json.Decode.string)
    |: (defaultTo [] <| "passed" := Json.Decode.list Json.Decode.string)
    |: (defaultTo False <| "trigger" := Json.Decode.bool)

decodeJobOutput : Json.Decode.Decoder JobOutput
decodeJobOutput =
  Json.Decode.succeed JobOutput
    |: ("name" := Json.Decode.string)
    |: ("resource" := Json.Decode.string)

dateFromSeconds : Float -> Date
dateFromSeconds =
  Date.fromTime << ((*) 1000)

decodePipeline : Json.Decode.Decoder Pipeline
decodePipeline =
  Json.Decode.succeed Pipeline
    |: ("name" := Json.Decode.string)
    |: ("url" := Json.Decode.string)
    |: ("paused" := Json.Decode.bool)
    |: ("public" := Json.Decode.bool)
    |: ("team_name" := Json.Decode.string)
    |: (defaultTo [] <| "groups" := (Json.Decode.list decodePipelineGroup))

decodePipelineGroup : Json.Decode.Decoder PipelineGroup
decodePipelineGroup =
  Json.Decode.succeed PipelineGroup
    |: ("name" := Json.Decode.string)
    |: (defaultTo [] <| "jobs" := Json.Decode.list Json.Decode.string)
    |: (defaultTo [] <| "resources" := Json.Decode.list Json.Decode.string)

decodeMetadata : Json.Decode.Decoder (List MetadataField)
decodeMetadata =
  Json.Decode.list decodeMetadataField

decodeMetadataField : Json.Decode.Decoder MetadataField
decodeMetadataField =
  Json.Decode.succeed MetadataField
    |: ("name" := Json.Decode.string)
    |: ("value" := Json.Decode.string)

decodeTeam : Json.Decode.Decoder Team
decodeTeam =
  Json.Decode.succeed Team
    |: ("id" := Json.Decode.int)
    |: ("name" := Json.Decode.string)

decodeUser : Json.Decode.Decoder User
decodeUser =
  Json.Decode.succeed User
    |: ("team" := decodeTeam)

decodeVersion : Json.Decode.Decoder Version
decodeVersion =
  Json.Decode.dict Json.Decode.string

defaultTo : a -> Json.Decode.Decoder a -> Json.Decode.Decoder a
defaultTo default =
  Json.Decode.map (Maybe.withDefault default) << Json.Decode.maybe
